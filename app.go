package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// App struct
type App struct {
	ctx          context.Context
	config       *Config
	currentPath  string
	history      []string
	historyIdx   int
	assigned     []*Entry
	recents      []string
	user         string
	userSetting  *userSetting
	session      string
	assignedOnly bool
	openCmd      string
}

// NewApp creates a new App application struct
func NewApp(cfg *Config) *App {
	return &App{
		config: cfg,
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	switch runtime.GOOS {
	case "windows":
		a.openCmd = "start"
	case "darwin":
		a.openCmd = "open"
	case "linux":
		a.openCmd = "xdg-open"
	default:
		log.Fatalf("unsupported os: %s", runtime.GOOS)
	}

	a.ctx = ctx
	a.GoTo("/")
}

func (a *App) Prepare() error {
	err := a.readSession()
	if err != nil {
		return fmt.Errorf("read session: %v", err)
	}
	if a.session == "" {
		return nil
	}
	err = a.getUserInfo()
	if err != nil {
		return fmt.Errorf("get user info: %v", err)
	}
	return nil
}

func (a *App) CurrentPath() string {
	return a.currentPath
}

type EntryResponse struct {
	Msg *Entry
	Err string
}

func (a *App) getEntry(path string) (*Entry, error) {
	resp, err := http.PostForm("https://imagvfx.com/api/get-entry", url.Values{
		"session": {a.session},
		"path":    {path},
	})
	if err != nil {
		return nil, err
	}
	r := EntryResponse{}
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

func (a *App) IsLeaf(path string) (bool, error) {
	e, err := a.getEntry(path)
	if err != nil {
		return false, err
	}
	return e.Type == a.config.LeafEntryType, nil
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
	Type string
	Path string
}

func (a *App) ListEntries() ([]string, error) {
	if a.assignedOnly {
		paths := a.subAssigned()
		sort.Strings(paths)
		return paths, nil
	}
	subs, err := a.subEntries(a.currentPath)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0)
	for _, e := range subs {
		paths = append(paths, e.Path)
	}
	sort.Strings(paths)
	return paths, nil
}

func (a *App) subEntries(path string) ([]*Entry, error) {
	resp, err := http.PostForm("https://imagvfx.com/api/sub-entries", url.Values{
		"session": {a.session},
		"path":    {path},
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
	return r.Msg, nil
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
	err = a.getUserInfo()
	if err != nil {
		return "", err
	}
	fmt.Println("login done")
	return a.user, nil
}

func (a *App) getUserInfo() error {
	err := a.writeSession()
	if err != nil {
		return fmt.Errorf("write session: %v", err)
	}
	err = a.GetUserSetting()
	if err != nil {
		return fmt.Errorf("user setting: %v", err)
	}
	err = a.searchAssigned()
	if err != nil {
		return fmt.Errorf("search assigned: %v", err)
	}
	return nil
}

func (a *App) OpenLoginPage(key string) error {
	cmd := exec.Command(a.openCmd, a.config.Host+"/login?app_session_key="+key)
	_, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	return nil
}

func (a *App) WaitLogin(key string) error {
	resp, err := http.PostForm(a.config.Host+"/api/app-login", url.Values{
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

func (a *App) writeSession() error {
	confd, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	err = os.MkdirAll(confd+"/field", 0755)
	if err != nil {
		return err
	}
	f, err := os.Create(confd + "/field/session")
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.WriteString(f, a.user+" "+a.session)
	if err != nil {
		return err
	}
	return nil
}

func (a *App) readSession() error {
	confd, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	f, err := os.Open(confd + "/field/session")
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	toks := strings.Split(string(data), " ")
	if len(toks) != 2 {
		return fmt.Errorf("invalid session")
	}
	a.user = toks[0]
	a.session = toks[1]
	return nil
}

func (a *App) removeSession() error {
	a.user = ""
	a.session = ""
	confd, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	err = os.Remove(confd + "/field/session")
	if err != nil {
		return err
	}
	return nil
}

func (a *App) arrangeRecentPaths(path string, at int) error {
	resp, err := http.PostForm(a.config.Host+"/api/update-user-setting", url.Values{
		"session":             {a.session},
		"update_recent_paths": {"1"},
		"path":                {path},
		"path_at":             {strconv.Itoa(at)},
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

func (a *App) RecentPaths() []string {
	if a.userSetting == nil {
		return []string{}
	}
	return a.userSetting.RecentPaths
}

func (a *App) addRecentPath(path string) error {
	err := a.arrangeRecentPaths(path, 0)
	if err != nil {
		return err
	}
	paths := make([]string, 0)
	for _, pth := range a.userSetting.RecentPaths {
		if path != pth {
			paths = append(paths, pth)
		}
	}
	a.userSetting.RecentPaths = append([]string{path}, paths...)
	return nil
}

func (a *App) Logout() error {
	a.assigned = nil
	err := a.removeSession()
	if err != nil {
		return err
	}
	return nil
}

func (a *App) SessionUser() string {
	return a.user
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

type Program struct {
	Name      string
	Ext       string
	CreateCmd []string
	OpenCmd   []string
}

func (a *App) Programs() []string {
	prog := make(map[string]bool, 0)
	for _, p := range a.config.Programs {
		prog[p.Name] = true
	}
	// User might have programs currently not defined for some reason.
	// Let's add those so user can remove if they want.
	for _, name := range a.userSetting.ProgramsInUse {
		prog[name] = true
	}
	progs := make([]string, 0, len(prog))
	for name := range prog {
		progs = append(progs, name)
	}
	sort.Strings(progs)
	return progs
}

func (a *App) IsValidProgram(prog string) bool {
	for _, p := range a.config.Programs {
		if p.Name == prog {
			return true
		}
	}
	return false
}

type userSetting struct {
	ProgramsInUse []string
	RecentPaths   []string
}

type forgeUserSettingResponse struct {
	Msg *userSetting
	Err string
}

func (a *App) GetUserSetting() error {
	resp, err := http.PostForm(a.config.Host+"/api/get-user-setting", url.Values{
		"session": {a.session},
		"user":    {a.user},
	})
	if err != nil {
		return err
	}
	r := forgeUserSettingResponse{}
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&r)
	if err != nil {
		return err
	}
	if r.Err != "" {
		return fmt.Errorf(r.Err)
	}
	a.userSetting = r.Msg
	return nil
}

func (a *App) ProgramsInUse() []string {
	if a.userSetting == nil {
		return []string{}
	}
	return a.userSetting.ProgramsInUse
}

type forgeAPIErrorResponse struct {
	Err string
}

func (a *App) arrangeProgramInUse(prog string, at int) error {
	resp, err := http.PostForm(a.config.Host+"/api/update-user-setting", url.Values{
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

// getEnv get value of a environment variable from `env`.
// It returns an empty string if it does not find the variable.
func getEnv(key string, env []string) string {
	for i := len(env) - 1; i >= 0; i-- {
		e := env[i]
		kv := strings.SplitN(e, "=", -1)
		if len(kv) != 2 {
			continue
		}
		k := strings.TrimSpace(kv[0])
		v := strings.TrimSpace(kv[1])
		if k == key {
			return v
		}
	}
	return ""
}

// evalEnvString fills environment variables of a string.
func evalEnvString(v string, env []string) string {
	reA := regexp.MustCompile(`[$]\w+`)
	reB := regexp.MustCompile(`[$][{]\w+[}]`)
	for {
		a := reA.FindStringIndex(v)
		b := reB.FindStringIndex(v)
		if a == nil && b == nil {
			break
		}
		var idxs []int
		if a != nil && b != nil {
			if a[0] < b[0] {
				idxs = a
			} else {
				idxs = b
			}
		} else {
			if a != nil {
				idxs = a
			}
			if b != nil {
				idxs = b
			}
		}
		s := idxs[0]
		e := idxs[1]
		pre := v[:s]
		post := v[e:]
		envk := v[s+1 : e]
		envk = strings.TrimPrefix(envk, "{")
		envk = strings.TrimSuffix(envk, "}")
		envv := getEnv(envk, env)
		v = pre + envv + post
	}
	return v
}

func (a *App) EntryEnvirons(path string) ([]Environ, error) {
	resp, err := http.PostForm(a.config.Host+"/api/entry-environs", url.Values{
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
	env := os.Environ()
	for _, e := range a.config.Envs {
		env = append(env, e)
	}
	envs, err := a.EntryEnvirons(path)
	if err != nil {
		return err
	}
	for _, e := range envs {
		env = append(env, e.Name+"="+e.Eval)
	}
	var pg *Program
	for _, p := range a.config.Programs {
		if p.Name == prog {
			pg = p
			break
		}
	}
	if pg == nil {
		return fmt.Errorf("not found program: %v", prog)
	}
	if len(pg.CreateCmd) == 0 {
		return fmt.Errorf("CreateCmd is not defined for program: %s", prog)
	}
	env = append(env, "ELEM="+name)
	env = append(env, "VER="+getEnv("NEW_VER", env))
	env = append(env, "EXT="+pg.Ext)
	scene := evalEnvString(a.config.Scene, env)
	err = os.MkdirAll(filepath.Dir(scene), 0755)
	if err != nil {
		return err
	}
	env = append(env, "SCENE="+scene)
	createCmd := append([]string{}, pg.CreateCmd...)
	for i, c := range createCmd {
		createCmd[i] = evalEnvString(c, env)
	}
	go func() {
		cmd := exec.Command(createCmd[0], createCmd[1:]...)
		cmd.Env = env
		b, err := cmd.CombinedOutput()
		out := string(b)
		fmt.Println(out)
		if err != nil {
			fmt.Println(err)
		}
	}()
	err = a.addRecentPath(path)
	if err != nil {
		return err
	}
	return nil
}

type Elem struct {
	Name     string
	Program  string
	Versions []string
}

func (a *App) ListElements() ([]*Elem, error) {
	path := a.currentPath
	env := os.Environ()
	for _, e := range a.config.Envs {
		env = append(env, e)
	}
	envs, err := a.EntryEnvirons(path)
	if err != nil {
		return nil, err
	}
	for _, e := range envs {
		env = append(env, e.Name+"="+e.Eval)
	}
	env = append(env, `ELEM=(?P<ELEM>\w+)`)
	env = append(env, `VER=(?P<VER>[vV]\d+)`)
	env = append(env, `EXT=(?P<EXT>\w+)`)
	scene := evalEnvString(a.config.Scene, env)
	sceneDir := filepath.Dir(scene)
	sceneName := filepath.Base(scene)
	reName, err := regexp.Compile(sceneName)
	if err != nil {
		return nil, err
	}
	files, err := os.ReadDir(sceneDir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		files = []os.DirEntry{}
	}
	programOf := make(map[string]*Program)
	for _, p := range a.config.Programs {
		programOf[p.Ext] = p
	}
	elem := make(map[string]*Elem, 0)
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		idxs := reName.FindStringSubmatchIndex(name)
		el := string(reName.ExpandString([]byte{}, "$ELEM", name, idxs))
		ver := string(reName.ExpandString([]byte{}, "$VER", name, idxs))
		ext := string(reName.ExpandString([]byte{}, "$EXT", name, idxs))
		p := programOf[ext]
		if p == nil {
			continue
		}
		e := elem[el]
		if e == nil {
			e = &Elem{
				Name:    el,
				Program: p.Name,
			}
		}
		e.Versions = append(e.Versions, ver)
		elem[el] = e
	}
	elems := make([]*Elem, 0, len(elem))
	for _, el := range elem {
		elems = append(elems, el)
	}
	sort.Slice(elems, func(i, j int) bool {
		cmp := strings.Compare(elems[i].Name, elems[j].Name)
		if cmp != 0 {
			return cmp < 0
		}
		cmp = strings.Compare(elems[i].Program, elems[j].Program)
		return cmp <= 0
	})
	return elems, nil
}

func (a *App) OpenScene(path, elem, ver, prog string) error {
	var pg *Program
	for _, p := range a.config.Programs {
		if p.Name == prog {
			pg = p
			break
		}
	}
	if pg == nil {
		return fmt.Errorf("program not found: %v", prog)
	}
	if len(pg.OpenCmd) == 0 {
		return fmt.Errorf("OpenCmd is not defined for program: %v", prog)
	}
	env := os.Environ()
	for _, e := range a.config.Envs {
		env = append(env, e)
	}
	envs, err := a.EntryEnvirons(path)
	if err != nil {
		return err
	}
	for _, e := range envs {
		env = append(env, e.Name+"="+e.Eval)
	}
	env = append(env, "ELEM="+elem)
	env = append(env, "VER="+ver)
	env = append(env, "EXT="+pg.Ext)
	env = append(env, "SCENE="+evalEnvString(a.config.Scene, env))
	openCmd := append([]string{}, pg.OpenCmd...)
	for i, c := range openCmd {
		openCmd[i] = evalEnvString(c, env)
	}
	go func() {
		cmd := exec.Command(openCmd[0], openCmd[1:]...)
		cmd.Env = env
		b, err := cmd.CombinedOutput()
		out := string(b)
		fmt.Println(out)
		if err != nil {
			fmt.Println(err)
		}
	}()
	err = a.addRecentPath(path)
	if err != nil {
		return err
	}
	return nil
}

func (a *App) OpenDir(path string) error {
	ent, err := a.getEntry(path)
	if err != nil {
		return err
	}
	dirTmpl, ok := a.config.Dir[ent.Type]
	if !ok {
		return fmt.Errorf("directory not defined for type %s", ent.Type)
	}
	env := os.Environ()
	for _, e := range a.config.Envs {
		env = append(env, e)
	}
	envs, err := a.EntryEnvirons(path)
	if err != nil {
		return err
	}
	for _, e := range envs {
		env = append(env, e.Name+"="+e.Eval)
	}
	dir := evalEnvString(dirTmpl, env)
	_, err = os.Stat(dir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			return err
		}
	}
	cmd := exec.Command(a.openCmd, dir)
	_, err = cmd.CombinedOutput()
	if err != nil {
		return err
	}
	return nil
}
