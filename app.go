package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

type Site struct {
	Host     string
	WorkDir  string
	WorkFile string
}

type Config struct {
	Site Site
}

func mustReadConfig() *Config {
	cfg := &Config{}
	_, err := toml.DecodeFile("./site.toml", &cfg)
	if err != nil {
		panic(err)
	}
	return cfg
}

// App struct
type App struct {
	ctx          context.Context
	config       *Config
	progsInUse   []string
	currentPath  string
	history      []string
	historyIdx   int
	assigned     []*Entry
	isLeaf       map[string]bool
	user         string
	session      string
	assignedOnly bool
}

// NewApp creates a new App application struct
func NewApp() *App {
	config := mustReadConfig()
	return &App{
		config: config,
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.GoTo("/")
}

func (a *App) CurrentPath() string {
	return a.currentPath
}

// GoTo goes to a path.
func (a *App) GoTo(pth string) {
	if pth == "" {
		return
	}
	if !path.IsAbs(pth) {
		pth = a.currentPath + "/" + pth
	}
	if pth != "/" && strings.HasSuffix(pth, "/") {
		pth = pth[:len(pth)-1]
	}
	if pth == a.currentPath {
		return
	}
	if len(a.history) > a.historyIdx+1 {
		a.history = a.history[:a.historyIdx+1]
	}
	a.history = append(a.history, pth)
	a.historyIdx = len(a.history) - 1
	a.currentPath = a.history[a.historyIdx]
}

// GoBack goes back to the previous path in history.
func (a *App) GoBack() string {
	if a.historyIdx != 0 {
		a.historyIdx--
	}
	a.currentPath = a.history[a.historyIdx]
	return a.currentPath
}

// GoForward goes again to the forward path in history.
func (a *App) GoForward() string {
	if a.historyIdx != len(a.history)-1 {
		a.historyIdx++
	}
	a.currentPath = a.history[a.historyIdx]
	return a.currentPath
}

func (a *App) SetAssignedOnly(only bool) {
	a.assignedOnly = only
}

type SearchResponse struct {
	Msg []*Entry
	Err string
}

type Entry struct {
	Path string
}

func (a *App) ListEntries() ([]string, error) {
	var paths []string
	if a.assignedOnly {
		paths = a.subAssigned()
	} else {
		subs, err := a.subEntries()
		if err != nil {
			return nil, err
		}
		paths = subs
	}
	sort.Strings(paths)
	return paths, nil
}

func (a *App) subEntries() ([]string, error) {
	resp, err := http.PostForm("https://imagvfx.com/api/sub-entries", url.Values{
		"session": {a.session},
		"path":    {a.currentPath},
	})
	if err != nil {
		return nil, err
	}
	r := SearchResponse{}
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&r)
	if err != nil {
		return nil, err
	}
	if r.Err != "" {
		return nil, fmt.Errorf(r.Err)
	}
	paths := make([]string, 0)
	for _, e := range r.Msg {
		paths = append(paths, e.Path)
	}
	return paths, nil
}

func (a *App) subAssigned() []string {
	dir := strings.TrimSuffix(a.currentPath, "/")
	paths := make([]string, 0)
	for _, e := range a.assigned {
		pth := strings.TrimSuffix(e.Path, "/")
		if e.Path == dir {
			continue
		}
		if !strings.HasPrefix(pth, dir+"/") {
			continue
		}
		rest := strings.TrimPrefix(pth, dir+"/")
		toks := strings.SplitN(rest, "/", 2)
		sub := dir + "/" + toks[0]
		paths = append(paths, sub)
	}
	return paths
}

func (a *App) searchAssigned() error {
	resp, err := http.PostForm("https://imagvfx.com/api/search-entries", url.Values{
		"session": {a.session},
		"from":    {"/"},
		"q":       {"assignee=" + a.user},
	})
	if err != nil {
		return err
	}
	r := SearchResponse{}
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&r)
	if err != nil {
		return err
	}
	if r.Err != "" {
		return fmt.Errorf(r.Err)
	}
	a.assigned = r.Msg
	a.isLeaf = make(map[string]bool)
	for _, e := range a.assigned {
		a.isLeaf[e.Path] = true
	}
	return nil
}

func (a *App) ClearEntries() {
	a.assigned = nil
}

type SessionInfo struct {
	User    string
	Session string
}

type LoginResponse struct {
	Msg SessionInfo
	Err string
}

// Login waits to login and return the logged in user name.
func (a *App) Login() (string, error) {
	key, err := GenerateRandomString(64)
	if err != nil {
		return "", err
	}
	err = a.OpenLoginPage(key)
	if err != nil {
		return "", err
	}
	err = a.WaitLogin(key)
	if err != nil {
		return "", err
	}
	err = a.searchAssigned()
	if err != nil {
		return "", err
	}
	fmt.Println("login done")
	return a.user, nil
}

func (a *App) OpenLoginPage(key string) error {
	cmd := exec.Command("open", a.config.Site.Host+"/login?app_session_key="+key)
	_, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	return nil
}

func (a *App) WaitLogin(key string) error {
	resp, err := http.PostForm("https://imagvfx.com/api/app-login", url.Values{
		"key": {key},
	})
	if err != nil {
		return err
	}
	r := LoginResponse{}
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&r)
	if err != nil {
		return err
	}
	if r.Err != "" {
		return fmt.Errorf(r.Err)
	}
	a.user = r.Msg.User
	a.session = r.Msg.Session
	return nil
}

func (a *App) Logout() {
	a.user = ""
	a.session = ""
	a.assigned = nil
	a.isLeaf = nil
}

func (a *App) SessionUser() string {
	return a.user
}

func (a *App) IsLeaf(path string) bool {
	return a.isLeaf[path]
}

func GenerateRandomString(n int) (string, error) {
	const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	ret := make([]byte, n)
	for i := 0; i < n; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return "", err
		}
		ret[i] = letters[num.Int64()]
	}
	return string(ret), nil
}

func (a *App) Programs() []string {
	// TODO: get available program list
	return []string{"blender", "nuke"}
}

type forgeUserSetting struct {
	ProgramsInUse []string
}

type forgeUserSettingResponse struct {
	Msg *forgeUserSetting
	Err string
}

func (a *App) getUserSetting() (*forgeUserSetting, error) {
	resp, err := http.PostForm(a.config.Site.Host+"/api/get-user-setting", url.Values{
		"session": {a.session},
		"user":    {a.user},
	})
	if err != nil {
		return nil, err
	}
	r := forgeUserSettingResponse{}
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&r)
	if err != nil {
		return nil, err
	}
	if r.Err != "" {
		return nil, fmt.Errorf(r.Err)
	}
	return r.Msg, nil
}

func (a *App) ProgramsInUse() ([]string, error) {
	setting, err := a.getUserSetting()
	if err != nil {
		return nil, err
	}
	return setting.ProgramsInUse, nil
}

type forgeAPIErrorResponse struct {
	Err string
}

func (a *App) arrangeProgramInUse(prog string, at int) error {
	resp, err := http.PostForm(a.config.Site.Host+"/api/update-user-setting", url.Values{
		"session":                {a.session},
		"update_programs_in_use": {"1"},
		"program":                {prog},
		"program_at":             {strconv.Itoa(at)},
	})
	if err != nil {
		return err
	}
	r := forgeAPIErrorResponse{}
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&r)
	if err != nil {
		return err
	}
	if r.Err != "" {
		return fmt.Errorf(r.Err)
	}
	return nil
}

func (a *App) AddProgramInUse(prog string, at int) error {
	return a.arrangeProgramInUse(prog, at)
}

func (a *App) RemoveProgramInUse(prog string) error {
	return a.arrangeProgramInUse(prog, -1)
}

type entryEnvironsResponse struct {
	Msg []Environ
	Err string
}

type Environ struct {
	Name string
	Eval string
}

func (a *App) EntryEnvirons(path string) ([]Environ, error) {
	resp, err := http.PostForm(a.config.Site.Host+"/api/entry-environs", url.Values{
		"session": {a.session},
		"path":    {path},
	})
	if err != nil {
		return nil, err
	}
	r := entryEnvironsResponse{}
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&r)
	if err != nil {
		return nil, err
	}
	if r.Err != "" {
		return nil, fmt.Errorf(r.Err)
	}
	return r.Msg, nil
}

func (a *App) NewElement(path, name, prog string) error {
	envs, err := a.EntryEnvirons(path)
	if err != nil {
		return err
	}
	// fill template
	workDir := a.config.Site.WorkDir
	workFile := a.config.Site.WorkFile
	runCommand := []string{"blender", workDir + "/" + workFile}
	fmt.Println(envs)
	fmt.Println(runCommand)
	return nil
}
