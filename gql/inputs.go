package gql

func DefaultCreateAppInput() CreateAppInput {
	return CreateAppInput{
		Runtime: "FIRECRACKER",
	}
}
