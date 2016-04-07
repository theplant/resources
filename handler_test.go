package resources_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	// Using postgres sql driver
	_ "github.com/lib/pq"

	"github.com/gin-gonic/gin"
	"github.com/jinzhu/gorm"

	"github.com/theplant/resources"
)

type Resource struct {
	gorm.Model

	UserID uint
	User   User
	Text   string `binding:"required"`
}

var resourceError error

func (r *Resource) BeforeSave(db *gorm.DB) error {
	return resourceError
}

// GetID part of resources.DBModel
func (r *Resource) GetID() uint {
	return r.ID
}

// OwnerID part of resources.DBModel
func (r *Resource) OwnerID() uint {
	return r.ParentID()
}

// SetOwner part of resources.DBModel
func (r *Resource) SetOwner(user resources.User) error {
	parent, ok := user.(resources.DBModel)
	if !ok {
		return errors.New("user must be a model")
	}
	return r.SetParent(parent)
}

// ParentID part of resources.DBModel
func (r *Resource) ParentID() uint {
	return r.UserID
}

// SetParent part of resources.DBModel
func (r *Resource) SetParent(model resources.DBModel) error {
	owner, ok := model.(*User)
	if !ok {
		return errors.New("owner isn't a user")
	}
	r.User = *owner
	r.UserID = owner.ID
	return nil
}

type User struct {
	gorm.Model
}

// GetID part of resources.DBModel
func (u *User) GetID() uint {
	return u.ID
}

// OwnerID part of resources.DBModel
func (u *User) OwnerID() uint {
	return u.ParentID()
}

// SetOwner part of resources.DBModel
func (u *User) SetOwner(user resources.User) error {
	return errors.New("user can't have a owner")
}

// ParentID part of resources.DBModel
func (u *User) ParentID() uint {
	panic("user don't have a parent id")
}

// SetParent part of resources.DBModel
func (u *User) SetParent(model resources.DBModel) error {
	return errors.New("user can't have a parent")
}

var (
	db *gorm.DB

	res resources.Resource

	router *gin.Engine
)

func openDB() {
	username := os.Getenv("DATABASE_POSTGRESQL_USERNAME")
	password := os.Getenv("DATABASE_POSTGRESQL_PASSWORD")
	database := os.Getenv("DATABASE_NAME_TEST")
	dbURL := fmt.Sprintf("postgres://%s:%s@localhost/%s?sslmode=disable", username, password, database)
	fmt.Println(dbURL)

	db_, err := gorm.Open("postgres", dbURL)
	if err != nil {
		panic(err)
	}

	db = db_
}

func TestMain(m *testing.M) {
	openDB()

	db.DropTableIfExists(&Resource{})
	db.AutoMigrate(&Resource{})
	db.DropTableIfExists(&User{})
	db.AutoMigrate(&User{})

	res = resources.New(db,
		func() resources.DBModel { return &Resource{} },
		func() interface{} { return []Resource{} },
		func(id uint) string { return fmt.Sprintf("/r/%d", id) })

	gin.SetMode(gin.TestMode)

	retCode := m.Run()
	os.Exit(retCode)
}

func TestPost(t *testing.T) {
	u := User{}
	assertNoErr(db.Save(&u).Error)

	req := mountOwnerParentHandler(t, &u, &u, res.Post)

	body := struct {
		Text string
	}{"text"}

	res := req(postBody(t, body))

	expected := http.StatusCreated
	if res.Code != expected {
		t.Fatalf("Error POSTing resource\nexpected %d, got %d: %v", expected, res.Code, res)
	}

	r := &Resource{}
	assertNoErr(db.Preload("User").Find(&r).Error) // Assuming there's only one resource in the DB...

	if r.UserID != u.ID {
		t.Fatalf("Didn't take ownership of resource\nexpected: '%v'\ngot:      '%v'", u.ID, r.UserID)
	}

	if r.Text != body.Text {
		t.Fatalf("Didn't set content of resource\nexpected: '%v'\ngot:      '%v'", body.Text, r.Text)
	}

	resRes := unmarshalBody(t, res)
	if resRes.User.ID != r.User.ID {
		t.Fatalf("Response with user relationship:\nexpected: '%v'\ngot:      '%v'", r.User.ID, resRes.User.ID)
	}

	// FIXME test location header

	res = req(postBody(t, struct{}{}))
	expected = resources.HTTPStatusUnprocessableEntity
	if res.Code != expected {
		t.Fatalf("Error POSTting resource with not enough data\nexpected %d, got %d: %v", expected, res.Code, res)
	}
}

func TestPostWithError(t *testing.T) {
	u := User{}
	assertNoErr(db.Save(&u).Error)

	req := mountOwnerParentHandler(t, &u, &u, res.Post)

	body := struct {
		Text string
	}{"text"}

	resourceError = errors.New("an error")
	defer clearResourceErrors()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("Error POSTing resource\ndidn't panic on error when saving resource")
		}
	}()

	req(postBody(t, body))
}

func TestPostWithAcceptableError(t *testing.T) {
	u := User{}
	assertNoErr(db.Save(&u).Error)

	req := mountOwnerParentHandler(t, &u, &u, res.Post)

	body := struct {
		Text string
	}{"text"}

	resourceError = errors.New("acceptable error")

	resources.AcceptableError = reflect.TypeOf(resourceError)
	defer clearResourceErrors()

	res := req(postBody(t, body))

	expected := resources.HTTPStatusUnprocessableEntity
	if res.Code != expected {
		t.Fatalf("Error POSTing resource\nexpected %d, got %d: %v", expected, res.Code, res)
	}
}

func TestGet(t *testing.T) {
	r := Resource{}
	assertNoErr(db.Save(&r).Error)

	req := mountResourceHandler(t, &r, res.Get)
	res := req(nil)

	expected := http.StatusOK
	if res.Code != expected {
		t.Fatalf("Error GETting resource\nexpected %d, got %d: %v", expected, res.Code, res)
	}

	expectedBody := jsonString(t, r)
	result := body(t, res)

	if expectedBody != result {
		t.Fatalf("Response differs:\nexpected: '%v'\ngot:      '%v'", expectedBody, result)
	}
}

func TestPatch(t *testing.T) {
	r := Resource{}
	assertNoErr(db.Save(&r).Error)

	req := mountResourceHandler(t, &r, res.Patch)

	update := struct {
		Text string
	}{"text"}

	res := req(postBody(t, update))

	expected := http.StatusOK
	if res.Code != expected {
		t.Fatalf("Error PATCHting resource\nexpected %d, got %d: %v", expected, res.Code, res)
	}

	reloaded := &Resource{}
	assertNoErr(db.Where("id = ?", r.ID).Find(&reloaded).Error)

	if reloaded.Text != update.Text {
		t.Fatalf("Didn't update resource\nexpected: '%v'\ngot:      '%v'", update.Text, reloaded.Text)
	}

	res = req(postBody(t, struct{}{}))
	expected = resources.HTTPStatusUnprocessableEntity
	if res.Code != expected {
		t.Fatalf("Error PATCHting resource with not enough data\nexpected %d, got %d: %v", expected, res.Code, res)
	}

}

func TestDelete(t *testing.T) {
	r := Resource{}
	assertNoErr(db.Save(&r).Error)

	req := mountResourceHandler(t, &r, res.Delete)
	resp := req(nil)

	expected := http.StatusNoContent
	if resp.Code != expected {
		t.Fatalf("Error deleting resource\nexpected %d, got %d: %v", expected, resp.Code, resp)
	}

	reloaded := &Resource{}
	err := db.Where("id = ?", r.ID).Find(&reloaded).Error
	if err != gorm.ErrRecordNotFound {
		t.Fatalf("Deleted resource still found in database")
	}
}

func mountResourceHandler(t *testing.T, r *Resource, handler resources.ModelHandler) func(body io.Reader) *httptest.ResponseRecorder {
	modelProvider := func(handler resources.ModelHandler) gin.HandlerFunc {
		return func(ctx *gin.Context) {
			handler(ctx, r)
		}
	}

	return mountHandler(t, modelProvider(handler))
}

func mountOwnerParentHandler(t *testing.T, u *User, p *User, handler resources.UserModelHandler) func(body io.Reader) *httptest.ResponseRecorder {
	ownerParentProvider := func(handler resources.UserModelHandler) gin.HandlerFunc {
		return func(ctx *gin.Context) {
			handler(ctx, u, u)
		}
	}

	return mountHandler(t, ownerParentProvider(handler))
}

func mountHandler(t *testing.T, handler func(*gin.Context)) func(body io.Reader) *httptest.ResponseRecorder {
	router = gin.New()
	path := "/test"
	// Path doesn't matter, model is provided by modelProvider
	router.GET(path, handler)

	return func(body io.Reader) *httptest.ResponseRecorder {
		return doRequest(t, "GET", path, body)
	}
}

func TestProvideModel(t *testing.T) {
	r := Resource{}
	assertNoErr(db.Save(&r).Error)

	router = gin.New()
	router.GET("/r/:id", res.ProvideModel(func(c *gin.Context, s resources.DBModel) {
		if s == nil {
			t.Fatal("ProvideModel passed a nil DBModel")
		}
		c.String(200, "OK")
	}))

	tests := []struct {
		Code int
		ID   string
	}{
		{200, strconv.FormatUint(uint64(r.ID), 10)},
		// We only have one resource, so this shouldn't exist
		{404, strconv.FormatUint(uint64(r.ID+1), 10)},
		{404, "0"},
		{404, "not-an-id"},
	}

	for _, test := range tests {
		path := fmt.Sprintf("/r/%s", test.ID)
		res := doRequest(t, "GET", path, nil)

		if res.Code != test.Code {
			t.Fatalf("Error finding resource at %s, expected %d, got %d: %v", path, test.Code, res.Code, res)
		}
	}
}

func doRequest(t *testing.T, method string, path string, body io.Reader) *httptest.ResponseRecorder {

	w := httptest.NewRecorder()
	req, err := http.NewRequest(method, path, body)
	assertNoErr(err)
	router.ServeHTTP(w, req)

	return w
}

func body(t *testing.T, res *httptest.ResponseRecorder) string {
	b, err := res.Body.ReadString(0)
	if err != nil && err != io.EOF {
		assertNoErr(err)
	}
	return strings.TrimSpace(b)
}

func unmarshalBody(t *testing.T, res *httptest.ResponseRecorder) *Resource {
	b, err := ioutil.ReadAll(res.Body)
	assertNoErr(err)

	resource := Resource{}
	err = json.Unmarshal(b, &resource)
	assertNoErr(err)
	return &resource
}

func postBody(t *testing.T, r interface{}) io.Reader {
	m, err := json.Marshal(r)
	assertNoErr(err)

	return strings.NewReader(string(m[:]))

}

func jsonString(t *testing.T, r interface{}) string {
	m, err := json.Marshal(r)
	assertNoErr(err)

	return strings.TrimSpace(string(m[:]))
}

func assertNoErr(err error) {
	if err != nil {
		panic(err)
	}
}

func clearResourceErrors() {
	resources.AcceptableError = nil
	resourceError = nil
}
