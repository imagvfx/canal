package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/imagvfx/forge"
)

type apiResponse struct {
	Msg interface{}
	Err string
}

func decodeAPIResponse(resp *http.Response, dest interface{}) error {
	r := apiResponse{Msg: dest}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(b, &r)
	if err != nil {
		return fmt.Errorf("%s: %s", err, b)
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

func getSessionUser(host, session string) (*forge.User, error) {
	if session == "" {
		return nil, fmt.Errorf("login please")
	}
	resp, err := http.PostForm("https://"+host+"/api/get-session-user", url.Values{
		"session": {session},
	})
	if err != nil {
		return nil, err
	}
	var u *forge.User
	err = decodeAPIResponse(resp, &u)
	if err != nil {
		return nil, err
	}
	return u, err
}

func getEntry(host, session, path string) (*forge.Entry, error) {
	if session == "" {
		return nil, fmt.Errorf("login please")
	}
	resp, err := http.PostForm("https://"+host+"/api/get-entry", url.Values{
		"session": {session},
		"path":    {path},
	})
	if err != nil {
		return nil, err
	}
	var ent *forge.Entry
	err = decodeAPIResponse(resp, &ent)
	if err != nil {
		return nil, err
	}
	return ent, err
}

func getThumbnail(host, session, path string) (*forge.Thumbnail, error) {
	if session == "" {
		return nil, fmt.Errorf("login please")
	}
	resp, err := http.PostForm("https://"+host+"/api/get-thumbnail", url.Values{
		"session": {session},
		"path":    {path},
	})
	if err != nil {
		return nil, err
	}
	var thumb *forge.Thumbnail
	err = decodeAPIResponse(resp, &thumb)
	if err != nil {
		return nil, err
	}
	return thumb, err
}

func getBaseEntryTypes(host, session string) ([]string, error) {
	if session == "" {
		return nil, fmt.Errorf("login please")
	}
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

func getGlobals(host, session, entType string) ([]*forge.Global, error) {
	if session == "" {
		return nil, fmt.Errorf("login please")
	}
	resp, err := http.PostForm("https://"+host+"/api/get-globals", url.Values{
		"session":    {session},
		"entry_type": {entType},
	})
	if err != nil {
		return nil, err
	}
	var globals []*forge.Global
	err = decodeAPIResponse(resp, &globals)
	if err != nil {
		return nil, err
	}
	return globals, nil
}

func subEntries(host, session, path string) ([]*forge.Entry, error) {
	if session == "" {
		return nil, fmt.Errorf("login please")
	}
	resp, err := http.PostForm("https://"+host+"/api/sub-entries", url.Values{
		"session": {session},
		"path":    {path},
	})
	if err != nil {
		return nil, err
	}
	var ents []*forge.Entry
	err = decodeAPIResponse(resp, &ents)
	if err != nil {
		return nil, err
	}
	return ents, nil
}

func parentEntries(host, session, path string) ([]*forge.Entry, error) {
	if session == "" {
		return nil, fmt.Errorf("login please")
	}
	resp, err := http.PostForm("https://"+host+"/api/parent-entries", url.Values{
		"session": {session},
		"path":    {path},
	})
	if err != nil {
		return nil, err
	}
	var parents []*forge.Entry
	err = decodeAPIResponse(resp, &parents)
	if err != nil {
		return nil, err
	}
	return parents, nil
}

func searchEntries(host, session, query string) ([]*forge.Entry, error) {
	if session == "" {
		return nil, fmt.Errorf("login please")
	}
	resp, err := http.PostForm("https://"+host+"/api/search-entries", url.Values{
		"session": {session},
		"from":    {"/"},
		"q":       {query},
	})
	if err != nil {
		return nil, err
	}
	var ents []*forge.Entry
	err = decodeAPIResponse(resp, &ents)
	if err != nil {
		return nil, err
	}
	return ents, nil
}

func ensureUserDataSection(host, session, user string) error {
	resp, err := http.PostForm("https://"+host+"/api/ensure-user-data-section", url.Values{
		"session": {session},
		"user":    {user},
		"section": {"canal"},
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

func getUserDataSection(host, session, user, section string) (*forge.UserDataSection, error) {
	if session == "" {
		return nil, fmt.Errorf("login please")
	}
	resp, err := http.PostForm("https://"+host+"/api/get-user-data-section", url.Values{
		"session": {session},
		"user":    {user},
		"section": {section},
	})
	if err != nil {
		return nil, err
	}
	var sec *forge.UserDataSection
	err = decodeAPIResponse(resp, &sec)
	if err != nil {
		return nil, err
	}
	return sec, nil
}

func setUserData(host, session, user, key, value string) error {
	if session == "" {
		return fmt.Errorf("login please")
	}
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
	if session == "" {
		return fmt.Errorf("login please")
	}
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
	if session == "" {
		return fmt.Errorf("login please")
	}
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

func getUserSetting(host, session, user string) (*forge.UserSetting, error) {
	if session == "" {
		return nil, fmt.Errorf("login please")
	}
	resp, err := http.PostForm("https://"+host+"/api/get-user-setting", url.Values{
		"session": {session},
		"user":    {user},
	})
	if err != nil {
		return nil, err
	}
	var setting *forge.UserSetting
	err = decodeAPIResponse(resp, &setting)
	if err != nil {
		return nil, err
	}
	return setting, nil
}

func entryEnvirons(host, session, path string) ([]*forge.Property, error) {
	if session == "" {
		return nil, fmt.Errorf("login please")
	}
	resp, err := http.PostForm("https://"+host+"/api/entry-environs", url.Values{
		"session": {session},
		"path":    {path},
	})
	if err != nil {
		return nil, err
	}
	var forgeEnv []*forge.Property
	err = decodeAPIResponse(resp, &forgeEnv)
	if err != nil {
		return nil, err
	}
	return forgeEnv, nil
}
