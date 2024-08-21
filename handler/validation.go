package handler

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-playground/validator"
)

type Validator struct {
	validator *validator.Validate
}

func date(fl validator.FieldLevel) bool {
	_, err := time.Parse("2006-01-02", fl.Field().String())
	return err == nil
}

func NewValidator() *Validator {
	validator := validator.New()
	if err := validator.RegisterValidation("date", date); err != nil {
		panic(err)
	}

	return &Validator{
		validator: validator,
	}
}

func (v Validator) Validate(data interface{}) error {
	errs := v.validator.Struct(data)
	if errs != nil && len(errs.(validator.ValidationErrors)) > 0 {

		errMsgs := make([]string, 0)
		for _, err := range errs.(validator.ValidationErrors) {
			errMsgs = append(errMsgs, fmt.Sprintf(
				"[%s]: '%v' | Needs to implement '%s'",
				err.Field(),
				err.Value(),
				err.Tag(),
			))
		}
		return fmt.Errorf(strings.Join(errMsgs, "\n"))
	}
	return nil
}
