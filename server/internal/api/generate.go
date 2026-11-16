package api

// The Go server types and strict-server interface, generated from openapi.yaml.
//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 -config oapi-codegen.yaml openapi.yaml

// The Roku app's requests and enums, from the same contract.
//go:generate go run ./internal/brsgen openapi.yaml ../../../clients/roku/source/generated/api.bs
