// Package resources provides a default implementation, as
// "specialised" `gin.HandlerFunc`s, of a RESTful (*eugh*) API for
// Gorm-backed models.
package resources

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/jinzhu/gorm"
)

const (
	// HTTPStatusUnprocessableEntity represents unprocesable entity http status
	HTTPStatusUnprocessableEntity = 422
)

var (
	// ErrRequestMissingMomentAttrs error represents missing attributes error when create or update a moment
	ErrRequestMissingMomentAttrs = errors.New("request requires attribute location")
)

var regexpID = regexp.MustCompile(`\d+`)

// DBModel defines an interface for ownership/authorisation when
// finding and creating DB models
type DBModel interface {
	OwnerID() uint
	SetOwnerID(uint)
	GetID() uint
}

// User defines an interface used by resource to "identify" a user,
// the key returned by `GetID` is used for comparison with
// `DBModel.OwnerID` for authorisation.
type User interface {
	GetID() uint
}

// UserHandler is a Gin handler function that also requires a User for
// correct operation. This kind of handler should be passed to a
// wrapper that will find a user somehow, and call the handler with
// the request context and the found user.
type UserHandler func(*gin.Context, User)

// ModelHandler is a Gin handler function that also requires a DBModel for
// correct operation. This kind of handler should be passed to a
// wrapper that will find a model somehow, and call the handler with
// the request context and the found model.
type ModelHandler func(*gin.Context, DBModel)

// Resource is a collection of specialised gin.HandlerFunc functions
// and handler wrappers for exporting Gorm-backed DB structs as HTTP
// API resources/endpoints.
type Resource struct {
	// Collection responds with:
	//
	// * 200 with JSON body of all resouces of this type owned by the
	//   given user
	Collection UserHandler

	// Post creates a single resource that will be owned by this user
	// by:
	//
	// 1. Binding the request body to the struct returned by `single`
	// 2. Setting the owner id of the struct to the collection owner.
	// 3. Saving the struct in the database.
	//
	// Responds with:
	// * 422 if binding failed
	// * 201 if saved to DB (setting `Location` header to result of
	//   calling `linker`)
	//
	// Panics on database error.
	Post UserHandler

	// Get responds with:
	//
	// * 200 with JSON body of serialised struct
	Get ModelHandler

	// Patch works similarly to Post, but updates given struct with
	// request.
	//
	// Responds with:
	// * 422 if binding failed
	// * 200 if DB updated
	//
	// Panics on database error.
	Patch ModelHandler

	// Delete deletes the struct from the database (supporting soft-delete)
	//
	// Responds with:
	// * 204
	//
	// Panics on database error.
	Delete ModelHandler

	// PublicFinder wraps a resource handler to provide the requested
	// DB model as a parameter to the function. DB model is looked up
	// via an `:id` param. It performs no authorisation.
	//
	// Responds with:
	// * 404 if DB model with given ID cannot be found
	// * Result of wrapped handler otherwise
	//
	// Panics on database error.
	PublicFinder func(ModelHandler) gin.HandlerFunc

	// PrivateFinder wraps a resource handler to provide the requested
	// DB model as a parameter to the function *if the model's owner
	// ID matches the user's ID*. DB model is looked up via an `:id`
	// param.
	//
	// Responds with:
	// * 401 if DB model owner ID != user ID
	// * Result of wrapped handler via PublicFinder otherwise
	PrivateFinder func(ModelHandler) UserHandler
}

// New creates a new resource that exposes the DBModel returned by
// `single` as a HTTP API. `collection` should return an array of the
// same type as `single`.
func New(db *gorm.DB, single func() DBModel, collection func() interface{}, linker func(id uint) string) Resource {

	r := Resource{}

	r.Collection = func(ctx *gin.Context, paramUser User) {
		c := collection()
		if err := db.Model(paramUser).Related(c).Error; err != nil && err != gorm.RecordNotFound {
			panic(err)
		}

		ctx.JSON(http.StatusOK, c)
	}

	r.Post = func(ctx *gin.Context, paramUser User) {
		s := single()
		if ctx.BindJSON(s) != nil {
			ctx.JSON(HTTPStatusUnprocessableEntity, errToJSON(ErrRequestMissingMomentAttrs))
			return
		}
		s.SetOwnerID(paramUser.GetID())

		if err := db.Create(s).Error; err != nil {
			panic(err)
		}

		ctx.Header("Location", absURL(ctx.Request, linker(s.GetID())))
		ctx.JSON(http.StatusCreated, s)
	}

	r.Get = func(ctx *gin.Context, s DBModel) {
		ctx.JSON(http.StatusOK, s)
	}

	r.Patch = func(ctx *gin.Context, s DBModel) {
		newS := single()
		if ctx.BindJSON(newS) != nil {
			ctx.JSON(HTTPStatusUnprocessableEntity, errToJSON(ErrRequestMissingMomentAttrs))
			return
		}

		if err := db.Model(s).Updates(newS).Error; err != nil {
			panic(err)
		}

		ctx.JSON(http.StatusOK, newS)
	}

	r.Delete = func(ctx *gin.Context, s DBModel) {
		if err := db.Delete(s).Error; err != nil {
			panic(err)
		}

		ctx.AbortWithStatus(http.StatusNoContent)
	}

	r.PublicFinder = func(handler ModelHandler) gin.HandlerFunc {
		return func(ctx *gin.Context) {
			id := ctx.Param("id")

			if !regexpID.MatchString(id) {
				ctx.AbortWithError(http.StatusNotFound, gorm.RecordNotFound)
				return
			}

			s := single()
			if err := db.Where("id = ?", id).First(s).Error; err == gorm.RecordNotFound {
				ctx.AbortWithError(http.StatusNotFound, gorm.RecordNotFound)
				return
			} else if err != nil {
				panic(err)
			}

			handler(ctx, s)
		}
	}

	r.PrivateFinder = func(handler ModelHandler) UserHandler {
		return func(ctx *gin.Context, u User) {
			r.PublicFinder(func(ctx *gin.Context, s DBModel) {
				if s.OwnerID() == u.GetID() {
					handler(ctx, s)
				} else {
					ctx.AbortWithStatus(http.StatusUnauthorized)
					return
				}
			})(ctx)
		}
	}

	return r
}

func errToJSON(err error) gin.H {
	return gin.H{"error": err.Error()}
}

func absURL(req *http.Request, path string) string {
	server := url.URL{
		Host: req.Host,
	}

	base := server.ResolveReference(req.URL)

	u, err := url.Parse(path)
	if err != nil {
		log.Panic(err)
	}

	return base.ResolveReference(u).String()
}
