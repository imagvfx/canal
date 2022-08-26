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
	"sync"
)

// App struct
type App struct {
	ctx    context.Context
	config *Config
	state  *State
	// hold cacheLock before modify cachedEnvs
	cacheLock   sync.Mutex
	cachedEnvs  map[string][]string
	globalLock  sync.Mutex
	global      map[string]map[string]*Global
	history     []string
	historyIdx  int
	assigned    []*Entry
	userSetting *userSetting
	openCmd     string
}

// NewApp creates a new App application struct
func NewApp(cfg *Config) *App {
	return &App{
		config: cfg,
		state:  &State{},
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
}

// Prepare prepares start up of the app gui.
// It is similar to startup, but I need separate method for functions
// those return error.
func (a *App) Prepare() error {
	toks := strings.Split(a.config.Host, "://")
	if len(toks) != 2 {
		return fmt.Errorf("invalid host: %v", a.config.Host)
	}
	a.state.Host = toks[1]
	err := a.readSession()
	if err != nil {
		return fmt.Errorf("read session: %v", err)
	}
	if a.state.Session == "" {
		return nil
	}
	err = a.Reload()
	if err != nil {
		return err
	}
	err = a.readOptions()
	if err != nil {
		return fmt.Errorf("read options: %v", err)
	}
	a.GoTo("/")
	return nil
}

type EntryResponse struct {
	Msg *Entry
	Err string
}

// getEntry gets entry info from host.
func (a *App) getEntry(path string) (*Entry, error) {
	resp, err := http.PostForm(a.config.Host+"/api/get-entry", url.Values{
		"session": {a.state.Session},
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

// GetEntry gets entry info from host.
func (a *App) GetEntry(path string) (*Entry, error) {
	ent, err := a.getEntry(path)
	if err != nil {
		return nil, err
	}
	return ent, nil
}

// IsLeaf checks whether a path is leaf.
func (a *App) IsLeaf(path string) (bool, error) {
	e, err := a.getEntry(path)
	if err != nil {
		return false, err
	}
	return e.Type == a.config.LeafEntryType, nil
}

type EntryTypesResponse struct {
	Msg []string
	Err string
}

func (a *App) getBaseEntryTypes() ([]string, error) {
	resp, err := http.PostForm(a.config.Host+"/api/get-base-entry-types", url.Values{
		"session": {a.state.Session},
	})
	if err != nil {
		return nil, err
	}
	r := EntryTypesResponse{}
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

type GlobalResponse struct {
	Msg []*Global
	Err string
}

type Global struct {
	Name  string
	Type  string
	Value string
}

func (a *App) getGlobals(entType string) ([]*Global, error) {
	resp, err := http.PostForm(a.config.Host+"/api/get-globals", url.Values{
		"session":    {a.state.Session},
		"entry_type": {entType},
	})
	if err != nil {
		return nil, err
	}
	r := GlobalResponse{}
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

func (a *App) ReloadGlobals() error {
	types, err := a.getBaseEntryTypes()
	if err != nil {
		return fmt.Errorf("get entry types: %v", err)
	}
	a.globalLock.Lock()
	defer a.globalLock.Unlock()
	a.global = make(map[string]map[string]*Global)
	for _, t := range types {
		globals, err := a.getGlobals(t)
		if err != nil {
			return fmt.Errorf("get globals: %v", err)
		}
		global := make(map[string]*Global)
		for _, g := range globals {
			global[g.Name] = g
		}
		a.global[t] = global
	}
	return nil
}

func (a *App) Global(entType, name string) *Global {
	a.globalLock.Lock()
	defer a.globalLock.Unlock()
	global := a.global[entType]
	if global == nil {
		return nil
	}
	return global[name]
}

func (a *App) StatusColor(entType, stat string) string {
	possibleStatus := a.Global(entType, "possible_status")
	if possibleStatus == nil {
		return ""
	}
	stats := strings.Split(possibleStatus.Value, " ")
	for _, s := range stats {
		toks := strings.Split(s, ":")
		if len(toks) != 2 {
			continue
		}
		name := strings.TrimSpace(toks[0])
		color := strings.TrimSpace(toks[1])
		if name == stat {
			return color
		}
	}
	return ""
}

type State struct {
	Host          string
	User          string
	Session       string
	Programs      []*Program
	ProgramsInUse []*Program
	RecentPaths   []string
	Path          string
	IsLeaf        bool
	Entries       []*Entry
	Elements      []*Elem
	Entry         *Entry
	ParentEntries []*Entry
	Dir           string
	DirExists     bool
	Options       *Options
}

func (a *App) State() *State {
	a.state.RecentPaths = a.RecentPaths()
	return a.state
}

// GoTo goes to a path.
func (a *App) GoTo(pth string) error {
	if pth == "" {
		return fmt.Errorf("please specify path to go")
	}
	if !path.IsAbs(pth) {
		pth = a.state.Path + "/" + pth
	}
	if pth != "/" && strings.HasSuffix(pth, "/") {
		pth = pth[:len(pth)-1]
	}
	if pth == a.state.Path {
		return nil
	}
	if len(a.history) > a.historyIdx+1 {
		a.history = a.history[:a.historyIdx+1]
	}
	a.history = append(a.history, pth)
	a.historyIdx = len(a.history) - 1
	a.state.Path = a.history[a.historyIdx]
	a.cacheLock.Lock()
	a.cachedEnvs = make(map[string][]string)
	a.cacheLock.Unlock()
	err := a.ReloadEntry()
	if err != nil {
		return err
	}
	return nil
}

// GoBack goes back to the previous path in history.
func (a *App) GoBack() error {
	if a.historyIdx != 0 {
		a.historyIdx--
	}
	a.state.Path = a.history[a.historyIdx]
	err := a.ReloadEntry()
	if err != nil {
		return err
	}
	return nil
}

// GoForward goes again to the forward path in history.
func (a *App) GoForward() error {
	if a.historyIdx != len(a.history)-1 {
		a.historyIdx++
	}
	a.state.Path = a.history[a.historyIdx]
	err := a.ReloadEntry()
	if err != nil {
		return err
	}
	return nil
}

// SetAssignedOnly set assignedOnly option enabled/disabled.
func (a *App) SetAssignedOnly(only bool) error {
	a.state.Options.AssignedOnly = only
	err := a.writeOptions()
	if err != nil {
		return err
	}
	return nil
}

type SearchResponse struct {
	Msg []*Entry
	Err string
}

// Entry is a entry info.
type Entry struct {
	Type     string
	Path     string
	Name     string
	Property map[string]*Property
}

// Property is a property an entry is holding.
type Property struct {
	Name  string
	Type  string
	Value string
	Eval  string
}

// String represents the property as string.
func (p *Property) String() string {
	return p.Eval
}

// ListEntries shows sub entries of an entry,
// it shows only paths to assigned entries when the options is enabled.
func (a *App) ListEntries(path string) ([]*Entry, error) {
	subs, err := a.subEntries(path)
	if err != nil {
		return nil, err
	}
	vis := make(map[string]bool)
	if a.state.Options.AssignedOnly {
		paths := a.subAssigned(path)
		for _, p := range paths {
			vis[p] = true
		}
	}
	ents := make([]*Entry, 0, len(subs))
	for _, e := range subs {
		if a.state.Options.AssignedOnly {
			if !vis[e.Path] {
				continue
			}
		}
		ents = append(ents, e)
	}
	return ents, nil
}

// ListAllEntries shows all sub entries of an entry.
func (a *App) ListAllEntries(path string) ([]*Entry, error) {
	ents, err := a.subEntries(path)
	if err != nil {
		return nil, err
	}
	return ents, nil
}

// subEntries get all sub entries of an entry from host.
func (a *App) subEntries(path string) ([]*Entry, error) {
	resp, err := http.PostForm(a.config.Host+"/api/sub-entries", url.Values{
		"session": {a.state.Session},
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
	ents := r.Msg
	sorters := a.entrySorters()
	sort.Slice(ents, func(i, j int) bool {
		cmp := strings.Compare(ents[i].Type, ents[j].Type)
		if cmp != 0 {
			return cmp < 0
		}
		sorter := sorters[ents[i].Type]
		dir := 1
		if sorter.Descending {
			dir = -1
		}
		cmp = func() int {
			prop := sorter.Property
			if prop == "" {
				return 0
			}
			ip := ents[i].Property[prop]
			jp := ents[j].Property[prop]
			if ip == nil {
				return -1
			}
			if jp == nil {
				return 1
			}
			cmp = strings.Compare(ip.Type, jp.Type)
			if cmp != 0 {
				return cmp
			}
			if ip.Value == "" {
				cmp++
			}
			if jp.Value == "" {
				cmp--
			}
			if cmp != 0 {
				// non-existing value shouldn't take priority over existing value.
				dir = 1
				return cmp
			}
			if ip.Type == "int" {
				iv, _ := strconv.Atoi(ip.Value)
				jv, _ := strconv.Atoi(jp.Value)
				if iv < jv {
					return -1
				}
				if iv > jv {
					return 1
				}
				return 0
			}
			return strings.Compare(ip.Value, jp.Value)
		}()
		if cmp != 0 {
			return dir*cmp < 0
		}
		cmp = strings.Compare(ents[i].Name, ents[j].Name)
		if cmp != 0 {
			return dir*cmp < 0
		}
		return true
	})
	return ents, nil
}

type Sorter struct {
	Property   string
	Descending bool
}

func (a *App) entrySorters() map[string]Sorter {
	sorters := make(map[string]Sorter)
	if a.userSetting == nil {
		return sorters
	}
	for typ, prop := range a.userSetting.EntryPageSortProperty {
		if prop == "" {
			continue
		}
		desc := false
		prefix := string(prop[0])
		if prefix == "+" {
		} else if prefix == "-" {
			desc = true
		} else {
			continue
		}
		prop = prop[1:]
		sorters[typ] = Sorter{Property: prop, Descending: desc}
	}
	return sorters
}

// subAssigned returns sub entry paths to assigned entries only.
func (a *App) subAssigned(path string) []string {
	dir := strings.TrimSuffix(path, "/")
	subs := make(map[string]bool)
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
		sub := toks[0]
		subs[sub] = true
	}
	paths := make([]string, 0)
	for sub := range subs {
		paths = append(paths, dir+"/"+sub)
	}
	sort.Strings(paths)
	return paths
}

// ParentEntries get parent entries of an entry.
func (a *App) ParentEntries(path string) ([]*Entry, error) {
	resp, err := http.PostForm(a.config.Host+"/api/parent-entries", url.Values{
		"session": {a.state.Session},
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

// ReloadAssigned searches entries from host those have logged in user as assignee.
func (a *App) ReloadAssigned() error {
	resp, err := http.PostForm(a.config.Host+"/api/search-entries", url.Values{
		"session": {a.state.Session},
		"from":    {"/"},
		"q":       {"assignee=" + a.state.User},
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

// ClearEntries clears assigned entries.
func (a *App) ClearEntries() {
	a.assigned = nil
}

// SessionInfo is a session info of logged in user.
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
	err = a.writeSession()
	if err != nil {
		return "", fmt.Errorf("write session: %v", err)
	}
	err = a.Reload()
	if err != nil {
		return "", err
	}
	fmt.Println("login done")
	return a.state.User, nil
}

func (a *App) Reload() error {
	err := a.ReloadGlobals()
	if err != nil {
		return fmt.Errorf("globals: %v", err)
	}
	err = a.ReloadUserSetting()
	if err != nil {
		return fmt.Errorf("user setting: %v", err)
	}
	err = a.ReloadAssigned()
	if err != nil {
		return fmt.Errorf("search assigned: %v", err)
	}
	return nil
}

func (a *App) ReloadEntry() error {
	path := a.state.Path
	leaf, err := a.IsLeaf(path)
	if err != nil {
		return err
	}
	a.state.IsLeaf = leaf
	entry, err := a.GetEntry(path)
	if err != nil {
		return err
	}
	a.state.Entry = entry
	parents, err := a.ParentEntries(path)
	if err != nil {
		return err
	}
	a.state.ParentEntries = parents
	// we only can have either entries or elements by design.
	a.state.Entries = []*Entry{}
	a.state.Elements = []*Elem{}
	if leaf {
		a.state.Elements, err = a.ListElements(path)
		if err != nil {
			return err
		}
	} else {
		a.state.Entries, err = a.ListEntries(path)
		if err != nil {
			return err
		}
	}
	dir, err := a.Dir(path)
	if err != nil {
		return err
	}
	a.state.Dir = dir
	dirExists, err := a.DirExists(path)
	if err != nil {
		return err
	}
	a.state.DirExists = dirExists
	return nil
}

// OpenLoginPage shows login page to user.
func (a *App) OpenLoginPage(key string) error {
	cmd := exec.Command(a.openCmd, a.config.Host+"/login?app_session_key="+key)
	_, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	return nil
}

// WaitLogin waits until the user log in.
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
	a.state.User = r.Msg.User
	a.state.Session = r.Msg.Session
	return nil
}

// readConfigData reads data from a config file.
func readConfigData(filename string) ([]byte, error) {
	confd, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(confd + "/canal/" + filename)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return []byte{}, nil
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// writeConfigData writes data to a config file.
func writeConfigData(filename string, data []byte) error {
	confd, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	err = os.MkdirAll(confd+"/canal", 0755)
	if err != nil {
		return err
	}
	f, err := os.Create(confd + "/canal/" + filename)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	if err != nil {
		return err
	}
	return nil
}

// removeConfigFile removes a config file.
func removeConfigFile(filename string) error {
	confd, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	err = os.Remove(confd + "/canal/" + filename)
	if err != nil {
		return err
	}
	return nil
}

// readSession reads session from a config file.
func (a *App) readSession() error {
	data, err := readConfigData("session")
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	toks := strings.Split(string(data), " ")
	if len(toks) != 2 {
		return fmt.Errorf("invalid session")
	}
	a.state.User = toks[0]
	a.state.Session = toks[1]
	return nil
}

// writeSession writes session to a config file.
func (a *App) writeSession() error {
	data := []byte(a.state.User + " " + a.state.Session)
	err := writeConfigData("session", data)
	if err != nil {
		return err
	}
	return nil
}

// removeSession removes sesson config file.
func (a *App) removeSession() error {
	a.state.User = ""
	a.state.Session = ""
	err := removeConfigFile("session")
	if err != nil {
		return err
	}
	return nil
}

// Options are options of the app those will remembered by config files.
// Note that options those are closely related with the user will remembered by host instead.
type Options struct {
	AssignedOnly bool
}

// readOptions reads options config file of the logged in user.
func (a *App) readOptions() error {
	if a.state.User == "" {
		return nil
	}
	data, err := readConfigData("options_" + a.state.User)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	err = json.Unmarshal(data, &a.state.Options)
	if err != nil {
		return err
	}
	return nil
}

// writeOptions writes options config file of the logged in user.
func (a *App) writeOptions() error {
	if a.state.User == "" {
		return nil
	}
	data, err := json.Marshal(&a.state.Options)
	if err != nil {
		return err
	}
	err = writeConfigData("options_"+a.state.User, data)
	if err != nil {
		return err
	}
	return nil
}

// arrangeRecentPaths insert/move/remove recent paths where user want.
func (a *App) arrangeRecentPaths(path string, at int) error {
	resp, err := http.PostForm(a.config.Host+"/api/update-user-setting", url.Values{
		"session":             {a.state.Session},
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

// RecentPaths returns recent paths of the logged in user.
func (a *App) RecentPaths() []string {
	if a.userSetting == nil {
		return []string{}
	}
	return a.userSetting.RecentPaths
}

// addRecentPath adds a path to head of recent paths.
// If the path has already in recent paths, it will move to head instead.
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

// Logout forgets session info of latest logged in user.
func (a *App) Logout() error {
	a.assigned = nil
	a.userSetting = nil
	err := a.removeSession()
	if err != nil {
		return err
	}
	return nil
}

// GenerateRandomString returns random string that has length 'n', using alpha-numeric characters.
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

// Program is a program info.
type Program struct {
	Name      string
	NotFound  bool
	Ext       string
	CreateCmd []string
	OpenCmd   []string
}

// Program returns a Program of given name.
// It will return error when not found the program or it is incompleted.
func (a *App) Program(prog string) (*Program, error) {
	for _, pg := range a.config.Programs {
		if pg.Name != prog {
			continue
		}
		if len(pg.CreateCmd) == 0 {
			return nil, fmt.Errorf("incomplete program: creation command is not defined: %s", prog)
		}
		if len(pg.OpenCmd) == 0 {
			return nil, fmt.Errorf("incomplete program: open command is not defined: %s", prog)
		}
		return pg, nil
	}
	return nil, fmt.Errorf("not found program: %s", prog)
}

// Programs returns programs in config, and legacy programs
// which user registred earlier when they were existed in previous config.
func (a *App) programs() []*Program {
	prog := make(map[string]*Program, 0)
	for _, p := range a.config.Programs {
		prog[p.Name] = p
	}
	if a.userSetting != nil {
		// User might have programs currently not defined for some reason.
		// Let's add those so user can remove if they want.
		for _, name := range a.userSetting.ProgramsInUse {
			if _, ok := prog[name]; !ok {
				prog[name] = &Program{Name: name, NotFound: true}
			}
		}
	}
	progs := make([]*Program, 0, len(prog))
	for _, p := range prog {
		progs = append(progs, p)
	}
	sort.Slice(progs, func(i, j int) bool { return progs[i].Name < progs[j].Name })
	return progs
}

// ProgramsInUse returns programs what user marked as in use.
// Note that it may have invalid programs.
func (a *App) programsInUse() []*Program {
	if a.userSetting == nil {
		return []*Program{}
	}
	prog := make(map[string]*Program, 0)
	for _, p := range a.config.Programs {
		prog[p.Name] = p
	}
	progs := make([]*Program, 0, len(prog))
	for _, name := range a.userSetting.ProgramsInUse {
		if _, ok := prog[name]; !ok {
			progs = append(progs, &Program{Name: name, NotFound: true})
		} else {
			progs = append(progs, prog[name])
		}
	}
	return progs
}

// userSetting is user setting saved in host that is related with this app.
type userSetting struct {
	ProgramsInUse         []string
	RecentPaths           []string
	EntryPageSortProperty map[string]string
}

type forgeUserSettingResponse struct {
	Msg *userSetting
	Err string
}

// ReloadUserSetting get user setting from host, and remember it.
func (a *App) ReloadUserSetting() error {
	resp, err := http.PostForm(a.config.Host+"/api/get-user-setting", url.Values{
		"session": {a.state.Session},
		"user":    {a.state.User},
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
	a.state.Programs = a.programs()
	a.state.ProgramsInUse = a.programsInUse()
	return nil
}

type forgeAPIErrorResponse struct {
	Err string
}

// arrangeProgramInUse insert/move/remove a in-use program to where user wants.
func (a *App) arrangeProgramInUse(prog string, at int) error {
	resp, err := http.PostForm(a.config.Host+"/api/update-user-setting", url.Values{
		"session":                {a.state.Session},
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

// AddProgramInUse adds a in-use program to where user wants.
func (a *App) AddProgramInUse(prog string, at int) error {
	return a.arrangeProgramInUse(prog, at)
}

// RemoveProgramInUse removes a in-use program.
func (a *App) RemoveProgramInUse(prog string) error {
	return a.arrangeProgramInUse(prog, -1)
}

type entryEnvironsResponse struct {
	Msg []forgeEnviron
	Err string
}

// forgeEnviron an environ defined for an entry retrieved from host.
type forgeEnviron struct {
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

// EntryEnvirons gets environs from an entry.
func (a *App) EntryEnvirons(path string) ([]string, error) {
	// check cached environs first to make only one query per path.
	// The cache is remained until user reloaded or moved to other entry.
	a.cacheLock.Lock()
	defer a.cacheLock.Unlock()
	env := a.cachedEnvs[path]
	if env != nil {
		return env, nil
	}
	resp, err := http.PostForm(a.config.Host+"/api/entry-environs", url.Values{
		"session": {a.state.Session},
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
	env = os.Environ()
	for _, e := range a.config.Envs {
		env = append(env, e)
	}
	for _, e := range r.Msg {
		env = append(env, e.Name+"="+e.Eval)
	}
	a.cachedEnvs[path] = env
	return env, nil
}

// NewElement creates a new element by creating a scene file.
func (a *App) NewElement(path, name, prog string) error {
	env, err := a.EntryEnvirons(path)
	if err != nil {
		return err
	}
	pg, err := a.Program(prog)
	if err != nil {
		return err
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
	cmd := exec.Command(createCmd[0], createCmd[1:]...)
	cmd.Env = env
	b, err := cmd.CombinedOutput()
	out := string(b)
	fmt.Println(out)
	if err != nil {
		fmt.Println(err)
	}
	err = a.addRecentPath(path)
	if err != nil {
		return err
	}
	return nil
}

// Elem is an element of a part.
// Elem in the app represents a bunch of files in a part directory which can be grouped by a naming rule.
type Elem struct {
	Name     string
	Program  string
	Versions []Version
}

// Version is a version of an element.
// Version in the app represents a file in a part directory that is in an element group.
type Version struct {
	Name  string
	Scene string
}

// ListElements returns elements of a part entry each of which holds versions as well.
func (a *App) ListElements(path string) ([]*Elem, error) {
	env, err := a.EntryEnvirons(path)
	if err != nil {
		return nil, err
	}
	env = append(env, `ELEM=(?P<ELEM>\w+)`)
	env = append(env, `VER=(?P<VER>[vV]\d+)`)
	env = append(env, `EXT=(?P<EXT>\w+)`)
	scene := evalEnvString(a.config.Scene, env)
	sceneDir, sceneName := filepath.Split(scene)
	reName, err := regexp.Compile("^" + sceneName + "$") // match as a whole
	if err != nil {
		return nil, err
	}
	files, err := os.ReadDir(sceneDir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return []*Elem{}, nil
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
		e := elem[el+"/"+p.Name]
		if e == nil {
			e = &Elem{
				Name:    el,
				Program: p.Name,
			}
		}
		v := Version{Name: ver, Scene: sceneDir + "/" + name}
		e.Versions = append(e.Versions, v)
		elem[el+"/"+p.Name] = e
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

func (a *App) LastVersionOfElement(path, elem, prog string) (string, error) {
	pg, err := a.Program(prog)
	if err != nil {
		return "", err
	}
	env, err := a.EntryEnvirons(path)
	if err != nil {
		return "", err
	}
	env = append(env, "ELEM="+elem)
	env = append(env, `VER=(?P<VER>[vV]\d+)`)
	env = append(env, "EXT="+pg.Ext)
	scene := evalEnvString(a.config.Scene, env)
	sceneDir, sceneName := filepath.Split(scene)
	reName, err := regexp.Compile("^" + sceneName + "$") // match as a whole
	if err != nil {
		return "", err
	}
	files, err := os.ReadDir(sceneDir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		return "", fmt.Errorf("not found scene directory: %v", sceneDir)
	}
	vers := make([]string, 0)
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		idxs := reName.FindStringSubmatchIndex(name)
		if idxs == nil {
			continue
		}
		ver := string(reName.ExpandString([]byte{}, "$VER", name, idxs))
		vers = append(vers, ver)
	}
	if len(vers) == 0 {
		return "", fmt.Errorf("element not exists: %s", elem)
	}
	sort.Slice(vers, func(i, j int) bool {
		a, _ := strconv.Atoi(vers[i][1:])
		b, _ := strconv.Atoi(vers[j][1:])
		return a > b
	})
	return vers[0], nil
}

// SceneFile returns scene filepath for given arguments combination.
func (a *App) SceneFile(path, elem, ver, prog string) (string, error) {
	if ver == "" {
		last, err := a.LastVersionOfElement(path, elem, prog)
		if err != nil {
			return "", err
		}
		ver = last
	}
	pg, err := a.Program(prog)
	if err != nil {
		return "", err
	}
	env, err := a.EntryEnvirons(path)
	if err != nil {
		return "", err
	}
	env = append(env, "ELEM="+elem)
	env = append(env, "VER="+ver)
	env = append(env, "EXT="+pg.Ext)
	scene := evalEnvString(a.config.Scene, env)
	return scene, nil
}

// OpenScene opens a scene that corresponds to the args (path, elem, ver, prog).
func (a *App) OpenScene(path, elem, ver, prog string) error {
	if ver == "" {
		last, err := a.LastVersionOfElement(path, elem, prog)
		if err != nil {
			return err
		}
		ver = last
	}
	pg, err := a.Program(prog)
	if err != nil {
		return err
	}
	env, err := a.EntryEnvirons(path)
	if err != nil {
		return err
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

// Dir returns directory path of an entry.
func (a *App) Dir(path string) (string, error) {
	ent, err := a.getEntry(path)
	if err != nil {
		return "", err
	}
	dirTmpl, ok := a.config.Dir[ent.Type]
	if !ok {
		return "", nil
	}
	env, err := a.EntryEnvirons(path)
	if err != nil {
		return "", err
	}
	dir := evalEnvString(dirTmpl, env)
	return dir, nil
}

// DirExists returns whether the directory path exists in filesystem.
func (a *App) DirExists(dir string) (bool, error) {
	_, err := os.Stat(dir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

// OpenDir opens a directory using native file browser of current OS.
func (a *App) OpenDir(dir string) error {
	_, err := os.Stat(dir)
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

// OpenURL opens a url page which shows information about the entry.
func (a *App) OpenURL(path string) error {
	cmd := exec.Command(a.openCmd, a.config.Host+path)
	_, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	return nil
}

// Parent returns parent path to an entry path.
func (a *App) Parent(pth string) string {
	return path.Dir(pth)
}
