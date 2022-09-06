package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type apiResponse struct {
	Msg interface{}
	Err string
}

func decodeAPIResponse(resp *http.Response, dest interface{}) error {
	r := apiResponse{Msg: dest}
	dec := json.NewDecoder(resp.Body)
	err := dec.Decode(&r)
	if err != nil {
		return err
	}
	if r.Err != "" {
		return fmt.Errorf(r.Err)
	}
	return nil
}
