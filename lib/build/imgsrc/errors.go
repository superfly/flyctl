package imgsrc

import "fmt"

type RegistryUnauthorizedError struct {
	Tag string
}

func (err *RegistryUnauthorizedError) Error() string {
	return fmt.Sprintf("you are not authorized to push \"%s\"", err.Tag)
}
