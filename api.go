package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
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

func appLogin(host, key string) (SessionInfo, error) {
	resp, err := http.PostForm("https://"+host+"/api/app-login", url.Values{
		"key": {key},
	})
	if err != nil {
		return SessionInfo{}, err
	}
	var info SessionInfo
	err = decodeAPIResponse(resp, &info)
	if err != nil {
		return SessionInfo{}, err
	}
	return info, nil
}

func getEntry(host, session, path string) (*Entry, error) {
	resp, err := http.PostForm("https://"+host+"/api/get-entry", url.Values{
		"session": {session},
		"path":    {path},
	})
	if err != nil {
		return nil, err
	}
	var ent *Entry
	err = decodeAPIResponse(resp, &ent)
	if err != nil {
		return nil, err
	}
	return ent, err
}

func getBaseEntryTypes(host, session string) ([]string, error) {
	resp, err := http.PostForm("https://"+host+"/api/get-base-entry-types", url.Values{
		"session": {session},
	})
	if err != nil {
		return nil, err
	}
	var types []string
	err = decodeAPIResponse(resp, &types)
	if err != nil {
		return nil, err
	}
	return types, nil
}

func getGlobals(host, session, entType string) ([]*Global, error) {
	resp, err := http.PostForm("https://"+host+"/api/get-globals", url.Values{
		"session":    {session},
		"entry_type": {entType},
	})
	if err != nil {
		return nil, err
	}
	var globals []*Global
	err = decodeAPIResponse(resp, &globals)
	if err != nil {
		return nil, err
	}
	return globals, nil
}

func subEntries(host, session, path string) ([]*Entry, error) {
	resp, err := http.PostForm("https://"+host+"/api/sub-entries", url.Values{
		"session": {session},
		"path":    {path},
	})
	if err != nil {
		return nil, err
	}
	var ents []*Entry
	err = decodeAPIResponse(resp, &ents)
	if err != nil {
		return nil, err
	}
	return ents, nil
}

func parentEntries(host, session, path string) ([]*Entry, error) {
	resp, err := http.PostForm("https://"+host+"/api/parent-entries", url.Values{
		"session": {session},
		"path":    {path},
	})
	if err != nil {
		return nil, err
	}
	var parents []*Entry
	err = decodeAPIResponse(resp, &parents)
	if err != nil {
		return nil, err
	}
	return parents, nil
}

func searchEntries(host, session, query string) ([]*Entry, error) {
	resp, err := http.PostForm("https://"+host+"/api/search-entries", url.Values{
		"session": {session},
		"from":    {"/"},
		"q":       {query},
	})
	if err != nil {
		return nil, err
	}
	var ents []*Entry
	err = decodeAPIResponse(resp, &ents)
	if err != nil {
		return nil, err
	}
	return ents, nil
}

func getUserDataSection(host, session, user string) (*UserDataSection, error) {
	resp, err := http.PostForm("https://"+host+"/api/get-user-data-section", url.Values{
		"session": {session},
		"user":    {user},
		"section": {"canal"},
	})
	if err != nil {
		return nil, err
	}
	var sec *UserDataSection
	err = decodeAPIResponse(resp, &sec)
	if err != nil {
		return nil, err
	}
	return sec, nil
}

func setUserData(host, session, user, key, value string) error {
	resp, err := http.PostForm("https://"+host+"/api/set-user-data", url.Values{
		"session": {session},
		"user":    {user},
		"section": {"canal"},
		"key":     {key},
		"value":   {value},
	})
	if err != nil {
		return err
	}
	err = decodeAPIResponse(resp, nil)
	if err != nil {
		return err
	}
	return nil
}

func arrangeRecentPaths(host, session, path string, at int) error {
	resp, err := http.PostForm("https://"+host+"/api/update-user-setting", url.Values{
		"session":             {session},
		"update_recent_paths": {"1"},
		"path":                {path},
		"path_at":             {strconv.Itoa(at)},
	})
	if err != nil {
		return err
	}
	err = decodeAPIResponse(resp, nil)
	if err != nil {
		return err
	}
	return nil
}

func arrangeProgramInUse(host, session, prog string, at int) error {
	resp, err := http.PostForm("https://"+host+"/api/update-user-setting", url.Values{
		"session":                {session},
		"update_programs_in_use": {"1"},
		"program":                {prog},
		"program_at":             {strconv.Itoa(at)},
	})
	if err != nil {
		return err
	}
	err = decodeAPIResponse(resp, nil)
	if err != nil {
		return err
	}
	return nil
}

func getUserSetting(host, session, user string) (*userSetting, error) {
	resp, err := http.PostForm("https://"+host+"/api/get-user-setting", url.Values{
		"session": {session},
		"user":    {user},
	})
	if err != nil {
		return nil, err
	}
	var setting *userSetting
	err = decodeAPIResponse(resp, &setting)
	if err != nil {
		return nil, err
	}
	return setting, nil
}

func entryEnvirons(host, session, path string) ([]forgeEnviron, error) {
	resp, err := http.PostForm("https://"+host+"/api/entry-environs", url.Values{
		"session": {session},
		"path":    {path},
	})
	if err != nil {
		return nil, err
	}
	var forgeEnv []forgeEnviron
	err = decodeAPIResponse(resp, &forgeEnv)
	if err != nil {
		return nil, err
	}
	return forgeEnv, nil
}
