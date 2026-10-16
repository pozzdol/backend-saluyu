package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

const maxBodyBytes = 1 << 20 // 1 MB

func init() {
	// Report fields by their JSON name ("img_url") instead of the Go name ("Image").
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		v.RegisterTagNameFunc(func(f reflect.StructField) string {
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "-" {
				return ""
			}
			return name
		})
	}
}

// bindJSON decodes the body, normalizes it (trim, lowercase email) and only then validates,
// so "   " cannot pass a min=3 rule. It writes the 400 response itself and returns false on failure.
func bindJSON(c *gin.Context, req any) bool {
	body := http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
	if err := json.NewDecoder(body).Decode(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return false
	}

	if n, ok := req.(interface{ Normalize() }); ok {
		n.Normalize()
	}

	if err := binding.Validator.ValidateStruct(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation failed", "fields": fieldErrors(err)})
		return false
	}
	return true
}

// fieldErrors turns validator output into {"email": "must be a valid email"}.
func fieldErrors(err error) map[string]string {
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		return map[string]string{"body": "is invalid"}
	}

	out := make(map[string]string, len(ve))
	for _, fe := range ve {
		out[fe.Field()] = fieldMessage(fe)
	}
	return out
}

func fieldMessage(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email"
	case "url":
		return "must be a valid URL"
	case "min":
		return "must be at least " + fe.Param() + " characters"
	case "max":
		return "must be at most " + fe.Param() + " characters"
	}
	return "is invalid"
}
