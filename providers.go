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

// The following LiftXX functions don't provide much value over
// defining the handler/processor directly, but they:
// 1. reduce complexity of the syntax required to define handlers/processors, and
// 2. signal that a function is intended as a handler/processor.

// LiftUserProvider parameterises a CPS-style User handler (that
// accepts the next action as a function parameter) into a function
// that accept the user handler as a parameter.
//
// This function doesn't provide much value over defining the handler
// as a processor directly, but it:
// 1. reduces complexity of the syntax required to define the processor, and
// 2. signals that the function is intended as a processor.
func LiftUserProvider(fn func(UserHandler, *gin.Context)) UserProvider {
	return func(handler UserHandler) gin.HandlerFunc {
		return func(ctx *gin.Context) {
			fn(handler, ctx)
		}
	}
}

// LiftModelProvider parameterises a CPS-style DBModel handler (that
// accepts the next action as a function parameter) into a function
// that accept the DBModel handler as a parameter.
func LiftModelProvider(fn func(ModelHandler, *gin.Context)) ModelProvider {
	return func(handler ModelHandler) gin.HandlerFunc {
		return func(ctx *gin.Context) {
			fn(handler, ctx)
		}
	}
}

// LiftModelProcessor parameterises a CPS-style DBModel processor (that
// accepts the handler as a function parameter) into a function
// that accept the provider as a parameter.
func LiftModelProcessor(fn func(ModelHandler, *gin.Context, DBModel)) func(ModelProvider) ModelProvider {
	return func(provider ModelProvider) ModelProvider {
		return func(accepter ModelHandler) gin.HandlerFunc {
			return provider(func(ctx *gin.Context, model DBModel) {
				fn(accepter, ctx, model)
			})
		}
	}
}

// LiftUserModelProcessor parameterises a CPS-style User + DBModel processor (that
// accepts the next action as a function parameter) into a function
// that accept the processor as a parameter.
func LiftUserModelProcessor(fn func(UserModelHandler, *gin.Context, User, DBModel)) func(UserModelProvider) UserModelProvider {
	return func(provider UserModelProvider) UserModelProvider {
		return func(accepter UserModelHandler) gin.HandlerFunc {
			return provider(func(ctx *gin.Context, user User, model DBModel) {
				fn(accepter, ctx, user, model)
			})
		}
	}
}
