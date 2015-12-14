package resources_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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
	Text   string `binding:"required"`
}

// OwnerID part of resources.DBModel
func (r *Resource) OwnerID() uint {
	return r.UserID
}

// SetOwnerID part of resources.DBModel
func (r *Resource) SetOwnerID(id uint) {
	r.UserID = id
}

// GetID part of resources.DBModel
func (r *Resource) GetID() uint {
	return r.ID
}

type User struct {
	gorm.Model
}

// GetID part of resources.User
func (u *User) GetID() uint {
	return u.ID
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

	db = &db_
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

	req := mountOwnerHandler(t, &u, res.Post)

	body := struct {
		Text string
	}{"text"}

	res := req(postBody(t, body))

	expected := http.StatusCreated
	if res.Code != expected {
		t.Fatalf("Error POSTing resource\nexpected %d, got %d: %v", expected, res.Code, res)
	}

	r := &Resource{}
	assertNoErr(db.Find(&r).Error) // Assuming there's only one resource in the DB...

	if r.UserID != u.ID {
		t.Fatalf("Didn't take ownership of resource\nexpected: '%v'\ngot:      '%v'", u.ID, r.UserID)
	}

	if r.Text != body.Text {
		t.Fatalf("Didn't set content of resource\nexpected: '%v'\ngot:      '%v'", body.Text, r.Text)
	}

	// FIXME test location header

	res = req(postBody(t, struct{}{}))
	expected = resources.HTTPStatusUnprocessableEntity
	if res.Code != expected {
		t.Fatalf("Error POSTting resource with not enough data\nexpected %d, got %d: %v", expected, res.Code, res)
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
	if err != gorm.RecordNotFound {
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

func mountOwnerHandler(t *testing.T, u *User, handler resources.UserHandler) func(body io.Reader) *httptest.ResponseRecorder {
	ownerProvider := func(handler resources.UserHandler) gin.HandlerFunc {
		return func(ctx *gin.Context) {
			handler(ctx, u)
		}
	}

	return mountHandler(t, ownerProvider(handler))
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

func TestPublicFinder(t *testing.T) {
	r := Resource{}
	assertNoErr(db.Save(&r).Error)

	router = gin.New()
	router.GET("/r/:id", res.PublicFinder(func(c *gin.Context, s resources.DBModel) {
		if s == nil {
			t.Fatal("PublicFinder passed a nil DBModel")
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

func TestPrivateFinder(t *testing.T) {
	userID := uint(10)

	r := Resource{UserID: userID}
	assertNoErr(db.Save(&r).Error)

	u := User{Model: gorm.Model{ID: userID}}
	assertNoErr(db.Save(&u).Error)

	userProvider := func(handler func(*gin.Context, resources.User)) gin.HandlerFunc {
		return func(ctx *gin.Context) {
			handler(ctx, &u)
		}
	}

	router = gin.New()
	router.GET("/r/:id", userProvider(res.PrivateFinder(func(c *gin.Context, s resources.DBModel) {
		if s == nil {
			t.Fatal("PrivateFinder passed a nil DBModel")
		}
		c.String(200, "OK")
	})))

	tests := []struct {
		Code   int
		UserID uint
	}{
		{200, r.UserID},
		{401, userID + 1},
		{401, 0},
	}

	path := fmt.Sprintf("/r/%d", r.ID)
	for _, test := range tests {
		u.ID = test.UserID

		res := doRequest(t, "GET", path, nil)

		if res.Code != test.Code {
			t.Fatalf("Error finding resource at %s requested by %d, owned by %d\nexpected %d, got %d: %v", path, test.UserID, r.UserID, test.Code, res.Code, res)
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
