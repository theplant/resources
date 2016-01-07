package resources

import "github.com/gin-gonic/gin"

// UserProvider is a function that knows how to "find" (or "provide")
// a User, given a request context. It doesn't do anything with the
// User beyond passing it to the User handler parameter
type UserProvider func(UserHandler) gin.HandlerFunc

// ModelProvider is a function that knows how to "find" (or "provide")
// a DBModel, given a request context. It doesn't do anything with the
// DBModel beyond passing it to the DBModel handler parameter
type ModelProvider func(ModelHandler) gin.HandlerFunc

// UserModelProvider is a function that knows how to "find" (or
// "provide") a User and a DBModel, given a request context. It
// doesn't do anything with the User or DBModel beyond passing it to
// the DBModel handler parameter
type UserModelProvider func(UserModelHandler) gin.HandlerFunc

// Merge will combine a User provider and DBModel provider into a
// single provider of both User and DBModel.
//
// Use case is something like:
// uP: provide user from app authentication mechanism
// mP: provide model from URL params
// => provider that pass give authed user and URL model to handler.
func Merge(uP UserProvider, mP ModelProvider) UserModelProvider {
	return func(accepter UserModelHandler) gin.HandlerFunc {
		return mP(func(ctx *gin.Context, model DBModel) {
			uP(func(ctx *gin.Context, user User) {
				accepter(ctx, user, model)
			})(ctx)
		})
	}
}

// UserAsModel converts a User provider to a DBModel provider.
//
// The returned handler will panic if the user cannot be converted to
// a DBModel.
func UserAsModel(mP UserProvider) ModelProvider {
	return func(accepter ModelHandler) gin.HandlerFunc {
		return mP(func(ctx *gin.Context, user User) {
			accepter(ctx, user.(DBModel))
		})
	}
}

// DiscardUser converts a User+DBModel provider into a DBModel provider by
// discarding the User.
func DiscardUser(p UserModelProvider) ModelProvider {
	return func(accepter ModelHandler) gin.HandlerFunc {
		return p(func(ctx *gin.Context, _ User, model DBModel) {
			accepter(ctx, model)
		})
	}
}

// CurryUserProvider curries a `func(UserHandler, *gin.Context)` into `func(UserHandler) gin.HandlerFunc` (ie. a `UserProvider`). Given:
//
//    func LoadUser(UserHandler, *ginContext) { ... }
//
// we can can curry it,
//
//    var curriedLoadUser func(UserHandler) gin.HandlerFunc := CurryUserProvider(LoadUser)
//
// and apply our handler:
//
//    var finalHandler gin.HandlerFunc = curriedLoadUser(userHandler)
//
// then we can pass the handler directly to a Gin RouterGroup:
//
//    ginHandler.GET("/url", finalHandler)
func CurryUserProvider(fn func(UserHandler, *gin.Context)) UserProvider {
	return func(handler UserHandler) gin.HandlerFunc {
		return func(ctx *gin.Context) {
			fn(handler, ctx)
		}
	}
}

// CurryModelProvider curries a DBModel provider in a similar way to
// CurryUserProvider
func CurryModelProvider(fn func(ModelHandler, *gin.Context)) ModelProvider {
	return func(handler ModelHandler) gin.HandlerFunc {
		return func(ctx *gin.Context) {
			fn(handler, ctx)
		}
	}
}

// CurryUserProcessor curries a User processor in a similar way to
// CurryUserProvider, but allows partial application of providers
// rather than handlers
func CurryUserProcessor(fn func(UserHandler, *gin.Context, User)) func(UserProvider) UserProvider {
	return func(provider UserProvider) UserProvider {
		return func(accepter UserHandler) gin.HandlerFunc {
			return provider(func(ctx *gin.Context, user User) {
				fn(accepter, ctx, user)
			})
		}
	}
}

// CurryModelProcessor curries a DBModel processor in a similar way to
// CurryUserProvider, but allows partial application of providers
// rather than handlers
func CurryModelProcessor(fn func(ModelHandler, *gin.Context, DBModel)) func(ModelProvider) ModelProvider {
	return func(provider ModelProvider) ModelProvider {
		return func(accepter ModelHandler) gin.HandlerFunc {
			return provider(func(ctx *gin.Context, model DBModel) {
				fn(accepter, ctx, model)
			})
		}
	}
}

// CurryUserModelProcessor curries a User + DBModel processor in a
// similar way to CurryUserProvider, but allows partial application of
// providers rather than handlers
func CurryUserModelProcessor(fn func(UserModelHandler, *gin.Context, User, DBModel)) func(UserModelProvider) UserModelProvider {
	return func(provider UserModelProvider) UserModelProvider {
		return func(accepter UserModelHandler) gin.HandlerFunc {
			return provider(func(ctx *gin.Context, user User, model DBModel) {
				fn(accepter, ctx, user, model)
			})
		}
	}
}
