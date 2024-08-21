package handler

import (
	"context"
	"encoding/json"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/schema"

	"net/http"

	"github.com/rs/zerolog/log"
)

type Controller interface {
	Routes() chi.Router
}

var dec = schema.NewDecoder()

type Error struct {
	Message string `json:"msg"`
}
type HandlerFunc[Req any, Res any] func(context.Context, Req) (*Res, error)

type Decoder[Req any] func(r *http.Request) (Req, error)

func write(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	err := json.NewEncoder(w).Encode(v)
	if err != nil {
		log.Error().Err(err).Msg("failed writing response")
		write(w, http.StatusInternalServerError, Error{Message: err.Error()})
	}
}

func JSON[T any](r *http.Request) (T, error) {
	request := new(T)
	err := json.NewDecoder(r.Body).Decode(request)
	if err != nil {
		return *request, err
	}
	validator := NewValidator()
	return *request, validator.Validate(request)
}

func Query[T any](r *http.Request) (T, error) {
	request := new(T)

	err := dec.Decode(request, r.URL.Query())
	if err != nil {
		return *request, err
	}
	validator := NewValidator()
	return *request, validator.Validate(request)
}

type ErrResponse struct {
	Err            error `json:"-"` // low-level runtime error
	HTTPStatusCode int   `json:"-"` // http response status code

	StatusText string `json:"status"`          // user-level status message
	AppCode    int64  `json:"code,omitempty"`  // application-specific error code
	ErrorText  string `json:"error,omitempty"` // application-level error message, for debugging
}

func Handler[Req any, Res any](h HandlerFunc[Req, Res], decoder Decoder[Req]) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req Req

		req, err := decoder(r)
		if err != nil {
			write(w, http.StatusBadRequest, &ErrResponse{
				Err:            err,
				HTTPStatusCode: 400,
				StatusText:     "Invalid request.",
				ErrorText:      err.Error(),
			})
			return
		}

		res, err := h(r.Context(), req)
		if err != nil {
			// TODO check the error and return the correct status code
			write(w, http.StatusInternalServerError, Error{Message: err.Error()})
			return
		}
		write(w, http.StatusAccepted, res)
	})
}
