package api

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

func SetupValidator() {
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		v.RegisterTagNameFunc(func(fld reflect.StructField) string {
			name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
			if name == "-" {
				return ""
			}
			return name
		})
	}
}

func getErrorMsg(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "This field is required"
	case "lte":
		return fmt.Sprintf("Should be less than or equal to %s", fe.Param())
	case "gte":
		return fmt.Sprintf("Should be greater than or equal to %s", fe.Param())
	case "email":
		return "Invalid email format"
	case "oneof":
		return fmt.Sprintf("Must be one of: %s", fe.Param())
	default:
		return "Unknown error"
	}
}

func bindAndValidate(ctx *gin.Context, req interface{}) bool {
	if err := ctx.ShouldBindJSON(req); err != nil {
		if verr, ok := err.(validator.ValidationErrors); ok {
			errors := make(map[string]string)
			for _, fe := range verr {
				errors[fe.Field()] = getErrorMsg(fe)
			}
			ctx.JSON(http.StatusBadRequest, gin.H{"errors": errors})
			return false
		}
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return false
	}
	return true
}

func bindAndValidateUri(ctx *gin.Context, req interface{}) bool {
	if err := ctx.ShouldBindUri(req); err != nil {
		if verr, ok := err.(validator.ValidationErrors); ok {
			errors := make(map[string]string)
			for _, fe := range verr {
				errors[fe.Field()] = getErrorMsg(fe)
			}
			ctx.JSON(http.StatusBadRequest, gin.H{"errors": errors})
			return false
		}
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return false
	}
	return true
}
