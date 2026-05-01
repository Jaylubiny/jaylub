package main

import (
	"fmt"
	"net/http"

	router "jaylub/internal/routers"
)


func main() {

	r1 := router.Basic()
	r2 := router.Company()

	go func() {
		fmt.Println("Server running on :8080")
		err := http.ListenAndServe(":8080", r1)
		if err != nil {
			fmt.Println(err)
		}
	}()

	fmt.Println("Server running on :8090")
	err := http.ListenAndServe(":8090", r2)
	if err != nil {
		fmt.Println(err)
	}
}
