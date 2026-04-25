package main

import(
	"fmt"
	"net/http"
	"jaylub/internal/routers"
)

func main() {
	
	fmt.Println("Starting server on port 8080...")

	r := router.NewRouter()

	http.ListenAndServe(":8080", r)

}
