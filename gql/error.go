package gql

import (
	"errors"
	"fmt"

	"github.com/vektah/gqlparser/gqlerror"
)

func IsErrorNotFound(err error) bool {
	fmt.Printf("Type of err: %T\n", err)
	var errList *gqlerror.List
	res := errors.As(errors.Unwrap(err), &errList)
	fmt.Println(res)
	return true
	// for i, err := range errList {
	// 	fmt.Printf("%d: %+v\n", i, err)
	// }

	// for _, err := range *errList {
	// 	fmt.Printf("%+v", err.Extensions["code"])
	// 	if err.Extensions["code"] == "NOT_FOUND" {
	// 		notFoundError = true
	// 		break
	// 	}
	// }
}
