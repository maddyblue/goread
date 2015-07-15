package app

import (
	"net/http"

	"github.com/mjibson/goread/_third_party/github.com/gorilla/mux"

	app "github.com/mjibson/goread"
)

func init() {
	router := mux.NewRouter()
	app.RegisterHandlers(router)
	http.Handle("/", router)
}
