package resources_test

import (
	"fmt"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jinzhu/gorm"

	"github.com/theplant/resources"
)

func TestMergeWithCurry(t *testing.T) {
	user := &User{gorm.Model{ID: 1}}
	model := &Resource{Model: gorm.Model{ID: 2}, UserID: user.ID}
	context := &gin.Context{}

	userProvider := resources.CurryUserProvider(func(handler resources.UserHandler, ctx *gin.Context) {
		if ctx != context {
			t.Fatal("provider called with unknown context")
		}
		handler(ctx, user)
	})

	modelProvider := resources.CurryModelProvider(func(handler resources.ModelHandler, ctx *gin.Context) {
		if ctx != context {
			t.Fatal("provider called with unknown context")
		}
		handler(ctx, model)
	})

	called := false
	handler := func(ctx *gin.Context, u resources.User, m resources.DBModel) {
		called = true
		if user != u || model != m {
			t.Fatal("handler called with different parameters")
		}
	}

	resources.Merge(userProvider, modelProvider)(handler)(context)

	if !called {
		t.Fatal("handler never called")
	}
}

func exampleProvider() {

	CurriedPreProcessModelUser := resources.CurryUserModelProcessor(func(accepter resources.UserModelHandler, ctx *gin.Context, user resources.User, model resources.DBModel) {
		// check model and user
		fmt.Println("Curried Pre-process model + user")
		accepter(ctx, user, model)
	})

	ProvideAuthUser := resources.CurryUserProvider(func(handler resources.UserHandler, ctx *gin.Context) {
		var u resources.User // comes from somewhere
		fmt.Println("Provide user")
		handler(ctx, u)
	})

	ProvideModel := resources.CurryModelProvider(func(handler resources.ModelHandler, ctx *gin.Context) {
		var model resources.DBModel // comes from somewhere
		fmt.Println("Provide model")
		handler(ctx, model)
	})

	PreProcessUser := resources.CurryUserProcessor(func(accepter resources.UserHandler, ctx *gin.Context, user resources.User) {
		// check user, then maybe call accepter
		fmt.Println("Pre-process user")
		accepter(ctx, user)
	})

	PreProcessModel := resources.CurryModelProcessor(func(accepter resources.ModelHandler, ctx *gin.Context, model resources.DBModel) {
		// check model, then maybe call accepter
		fmt.Println("Pre-process model")
		accepter(ctx, model)
	})

	PreProcessModelUser := resources.CurryUserModelProcessor(func(accepter resources.UserModelHandler, ctx *gin.Context, user resources.User, model resources.DBModel) {
		// check model and user
		fmt.Println("Pre-process model + user")
		accepter(ctx, user, model)
	})

	AcceptModel := func(ctx *gin.Context, model resources.DBModel) {
		// do something with model
		fmt.Println("Accept model")
	}

	AcceptUserModel := func(ctx *gin.Context, user resources.User, model resources.DBModel) {
		// do something with model
		fmt.Println("Accept user + model")
	}

	chain := CurriedPreProcessModelUser(PreProcessModelUser(resources.Merge(PreProcessUser(ProvideAuthUser), PreProcessModel(ProvideModel))))

	chain(AcceptUserModel)(nil)

	resources.DiscardUser(chain)(AcceptModel)(nil)
	// Output:
	// Provide model
	// Pre-process model
	// Provide user
	// Pre-process user
	// Pre-process model + user
	// Curried Pre-process model + user
	// Accept user + model
	// Provide model
	// Pre-process model
	// Provide user
	// Pre-process user
	// Pre-process model + user
	// Curried Pre-process model + user
	// Accept model
}
