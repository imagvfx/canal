package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/imagvfx/forge"
	wails "github.com/wailsapp/wails/v2/pkg/runtime"
)

type ElemNotExistError struct {
	elem string
}

func (e *ElemNotExistError) Error() string {
	return fmt.Sprintf("element does not exist: %s", e.elem)
}

// App struct
type App struct {
	ctx     context.Context
	config  *Config
	host    string
	user    string
	session string
	program map[string]*Program
	state   *State
	// hold cacheLock before modify cachedEnvs
	cacheLock     sync.Mutex
	cachedEnvs    map[string][]string
	globalLock    sync.Mutex
	global        map[string]map[string]*forge.Global
	thumbnail     map[string]*forge.Thumbnail
	thumbnailLock sync.Mutex
	history       []string
	historyIdx    int
	assigned      []*forge.Entry
	entrySorters  map[string]Sorter
}

// NewApp creates a new App application struct
func NewApp(cfg *Config) *App {
	program := make(map[string]*Program)
	for _, pg := range cfg.Programs {
		program[pg.Name] = pg
	}
	thumbnail := make(map[string]*forge.Thumbnail)
	return &App{
		config:    cfg,
		host:      cfg.Host,
		program:   program,
		thumbnail: thumbnail,
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// Prepare prepares start up of the app gui.
// It is similar to startup, but I need separate method for functions
// those return error.
func (a *App) Prepare() error {
	err := a.readSession()
	if err != nil {
		return fmt.Errorf("read session: %v", err)
	}
	err = a.afterLogin()
	if err != nil {
		return err
	}
	return nil
}

// ReloadBase reloads base information needed by the app from the host.
func (a *App) ReloadBase(force bool) error {
	if !force && a.state.baseLoaded {
		return nil
	}
	var err error
	a.state.baseLoaded = false
	if a.session == "" {
		return nil
	}
	a.state.Host = a.host
	a.state.User, err = getSessionUser(a.host, a.session)
	if err != nil {
		// may be the session is expired, remove the session then try again.
		err := a.removeSession()
		if err != nil {
			return err
		}
		a.state.User, err = getSessionUser(a.host, a.session)
		if err != nil {
			return err
		}
		return fmt.Errorf("session user: %v", err)
	}
	progs := make([]string, 0, len(a.program))
	for _, p := range a.program {
		progs = append(progs, p.Name)
	}
	sort.Strings(progs)
	a.state.Programs = progs
	err = a.ReloadGlobals()
	if err != nil {
		return fmt.Errorf("globals: %v", err)
	}
	err = a.ReloadUserSetting()
	if err != nil {
		return fmt.Errorf("user setting: %v", err)
	}
	err = a.ReloadUserData()
	if err != nil {
		return fmt.Errorf("user data: %v", err)
	}
	err = a.ReloadAssigned()
	if err != nil {
		return fmt.Errorf("search assigned: %v", err)
	}
	a.state.baseLoaded = true
	return nil
}

// GetEntry gets entry info from host.
func (a *App) GetEntry(path string) (*forge.Entry, error) {
	ent, err := getEntry(a.host, a.session, path)
	if err != nil {
		return nil, err
	}
	return ent, nil
}

func (a *App) GetThumbnail(path string) (*forge.Thumbnail, error) {
	a.thumbnailLock.Lock()
	defer a.thumbnailLock.Unlock()
	thumb := a.thumbnail[path]
	if thumb != nil {
		return thumb, nil
	}
	var thumbEnt *forge.Entry
	ent, err := a.GetEntry(path)
	if err != nil {
		return nil, err
	}
	if ent.HasThumbnail {
		thumbEnt = ent
	} else {
		parents, err := a.ParentEntries(path)
		if err != nil {
			return nil, err
		}
		for i := len(parents) - 1; i >= 0; i-- {
			ent = parents[i]
			if ent.HasThumbnail {
				thumbEnt = ent
				break
			}
		}
	}
	if thumbEnt == nil {
		return nil, fmt.Errorf("couldn't find thumbnail: %v", path)
	}
	thumb = a.thumbnail[thumbEnt.Path]
	if thumb != nil {
		return thumb, nil
	}
	thumb, err = getThumbnail(a.host, a.session, thumbEnt.Path)
	if err != nil {
		return nil, err
	}
	a.thumbnail[thumbEnt.Path] = thumb
	a.thumbnail[path] = thumb
	return thumb, nil
}

func (a *App) ReloadGlobals() error {
	types, err := getBaseEntryTypes(a.host, a.session)
	if err != nil {
		return fmt.Errorf("get entry types: %v", err)
	}
	a.globalLock.Lock()
	defer a.globalLock.Unlock()
	a.global = make(map[string]map[string]*forge.Global)
	for _, t := range types {
		globals, err := getGlobals(a.host, a.session, t)
		if err != nil {
			return fmt.Errorf("get globals: %v", err)
		}
		global := make(map[string]*forge.Global)
		for _, g := range globals {
			global[g.Name] = g
		}
		a.global[t] = global
	}
	return nil
}

func (a *App) Global(entType, name string) *forge.Global {
	a.globalLock.Lock()
	defer a.globalLock.Unlock()
	global := a.global[entType]
	if global == nil {
		return nil
	}
	return global[name]
}

func (a *App) SortEntryProperties(entProps []string, entType string) []string {
	filter := a.Global(entType, "property_filter")
	if filter == nil {
		return entProps
	}
	order := make(map[string]int)
	filterProps := strings.Fields(filter.Value)
	for i, p := range filterProps {
		order[p] = i
	}
	sort.Slice(entProps, func(i, j int) bool {
		ip := entProps[i]
		jp := entProps[j]
		io, ok := order[ip]
		if !ok {
			io = len(entProps)
		}
		jo, ok := order[jp]
		if !ok {
			jo = len(entProps)
		}
		if io == jo {
			return ip < jp
		}
		return io < jo
	})
	return entProps
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
	// Loaded indicates whether last ReloadBase was successful.
	baseLoaded        bool
	Host              string
	User              *forge.User
	Programs          []string
	LegacyPrograms    []string
	ProgramsInUse     []string
	RecentPaths       []string
	Path              string
	AtLeaf            bool
	Entries           []*forge.Entry
	Elements          []*Elem
	Entry             *forge.Entry
	ParentEntries     []*forge.Entry
	Dir               string
	DirExists         bool
	Options           Options
	ExposedProperties map[string][]string
}

func (a *App) State() *State {
	return a.state
}

func (a *App) newState() *State {
	return &State{
		Host:              a.host,
		Path:              "",
		Programs:          make([]string, 0),
		LegacyPrograms:    make([]string, 0),
		ProgramsInUse:     make([]string, 0),
		RecentPaths:       make([]string, 0),
		Entries:           make([]*forge.Entry, 0),
		Elements:          make([]*Elem, 0),
		ParentEntries:     make([]*forge.Entry, 0),
		ExposedProperties: make(map[string][]string),
	}
}

// GoTo goes to a path.
func (a *App) GoTo(pth string) error {
	if pth == "" {
		return fmt.Errorf("please specify path to go")
	}
	if pth != "/" && strings.HasSuffix(pth, "/") {
		pth = pth[:len(pth)-1]
	}
	if pth == a.state.Path {
		return nil
	}
	entry, err := a.GetEntry(pth)
	if err != nil {
		return err
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
	err = a.loadEntry(entry)
	if err != nil {
		return err
	}
	return nil
}

// GoBack goes back to the previous path in history.
func (a *App) GoBack() error {
	if a.historyIdx < 0 {
		// there is race condition I couldn't fix unfortunately
		a.historyIdx = 0
	}
	if a.historyIdx <= 0 {
		return fmt.Errorf("no previous entry")
	}
	pth := a.history[a.historyIdx-1]
	entry, err := a.GetEntry(pth)
	if err != nil {
		return err
	}
	a.historyIdx--
	a.state.Path = pth
	err = a.loadEntry(entry)
	if err != nil {
		return err
	}
	return nil
}

// GoForward goes again to the forward path in history.
func (a *App) GoForward() error {
	if a.historyIdx > len(a.history)-1 {
		// there is race condition I couldn't fix unfortunately
		a.historyIdx = len(a.history) - 1
	}
	if a.historyIdx >= len(a.history)-1 {
		return fmt.Errorf("no next entry")
	}
	pth := a.history[a.historyIdx+1]
	entry, err := a.GetEntry(pth)
	if err != nil {
		return err
	}
	a.historyIdx++
	a.state.Path = pth
	err = a.loadEntry(entry)
	if err != nil {
		return err
	}
	return nil
}

// SetAssignedOnly set assignedOnly option enabled/disabled.
func (a *App) SetAssignedOnly(only bool) error {
	a.state.Options.AssignedOnly = only
	value, err := json.Marshal(only)
	if err != nil {
		return err
	}
	err = setUserData(a.host, a.session, a.user, "options.assigned_only", string(value))
	if err != nil {
		return err
	}
	return nil
}

// ListEntries shows sub entries of an entry,
// it shows only paths to assigned entries when the options is enabled.
func (a *App) ListEntries(path string) ([]*forge.Entry, error) {
	subs, err := a.ListAllEntries(path)
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
	ents := make([]*forge.Entry, 0, len(subs))
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
func (a *App) ListAllEntries(path string) ([]*forge.Entry, error) {
	ents, err := subEntries(a.host, a.session, path)
	if err != nil {
		return nil, err
	}
	sort.Slice(ents, func(i, j int) bool {
		cmp := strings.Compare(ents[i].Type, ents[j].Type)
		if cmp != 0 {
			return cmp < 0
		}
		sorter := a.entrySorters[ents[i].Type]
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
		cmp = strings.Compare(ents[i].Name(), ents[j].Name())
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

func (a *App) makeEntrySorters(entryPageSortProperty map[string]string) map[string]Sorter {
	sorters := make(map[string]Sorter)
	for typ, prop := range entryPageSortProperty {
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
func (a *App) ParentEntries(path string) ([]*forge.Entry, error) {
	parents, err := parentEntries(a.host, a.session, path)
	if err != nil {
		return nil, err
	}
	return parents, nil
}

// ReloadAssigned searches entries from host those have logged in user as assignee.
func (a *App) ReloadAssigned() error {
	query := "assignee=" + a.user
	ents, err := searchEntries(a.host, a.session, query)
	if err != nil {
		return err
	}
	a.assigned = ents
	return nil
}

// SessionInfo is a session info of logged in user.
type SessionInfo struct {
	User    string
	Session string
}

// Login waits to login and return the logged in user name.
func (a *App) Login() (string, error) {
	key, err := GenerateRandomString(64)
	if err != nil {
		return "", err
	}
	err = a.OpenLoginPage(key)
	if err != nil {
		return "", fmt.Errorf("open login page: %v", err)
	}
	err = a.WaitLogin(key)
	if err != nil {
		return "", fmt.Errorf("wait login: %v", err)
	}
	err = a.writeSession()
	if err != nil {
		return "", fmt.Errorf("write session: %v", err)
	}
	err = a.afterLogin()
	if err != nil {
		return "", err
	}
	fmt.Println("login done")
	return a.user, nil
}

func (a *App) afterLogin() error {
	user, err := getSessionUser(a.host, a.session)
	if err != nil {
		return fmt.Errorf("get session user: %v", err)
	}
	a.user = user.Name
	err = ensureUserDataSection(a.host, a.session, a.user)
	if err != nil {
		return fmt.Errorf("ensure user data section: %v", err)
	}
	a.state = a.newState()
	err = a.ReloadBase(true)
	if err != nil {
		return fmt.Errorf("reload base: %v", err)
	}
	path := "/"
	if len(a.state.RecentPaths) != 0 {
		path = a.state.RecentPaths[0]
	}
	err = a.GoTo(path) // at least one page needed in history
	if err != nil {
		// the entry might be deleted, or in an unrecoverable state.
		// start from root instead.
		err = a.GoTo("/")
		if err != nil {
			return err
		}
		return nil
	}
	return nil
}

func (a *App) ReloadEntry() error {
	pth := "/"
	if len(a.history) != 0 {
		pth = a.history[a.historyIdx]
	}
	entry, err := a.GetEntry(pth)
	if err != nil {
		return err
	}
	return a.loadEntry(entry)
}

func (a *App) loadEntry(entry *forge.Entry) error {
	if entry == nil {
		return fmt.Errorf("nil entry")
	}
	err := a.ReloadBase(false)
	if err != nil {
		return err
	}
	path := a.state.Path
	a.state.Entry = entry
	a.state.AtLeaf = false
	if a.state.Entry.Type == a.config.LeafEntryType {
		a.state.AtLeaf = true
	}
	a.state.ParentEntries, err = a.ParentEntries(path)
	if err != nil {
		return err
	}
	a.state.Entries = []*forge.Entry{}
	a.state.Elements = []*Elem{}
	// we only can have either entries or elements by design.
	if a.state.AtLeaf {
		a.state.Elements, err = a.ListElements(path)
	} else {
		a.state.Entries, err = a.ListEntries(path)
	}
	if err != nil {
		return err
	}
	return nil
}

// OpenLoginPage shows login page to user.
func (a *App) OpenLoginPage(key string) error {
	return openPath("https://" + a.host + "/login?app_session_key=" + key)
}

// WaitLogin waits until the user log in.
func (a *App) WaitLogin(key string) error {
	info, err := appLogin(a.host, key)
	if err != nil {
		return err
	}
	a.user = info.User
	a.session = info.Session
	return nil
}

// readSession reads session from a config file.
func (a *App) readSession() error {
	data, err := readConfigFile("forge/session")
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	a.session = strings.TrimSpace(string(data))
	return nil
}

// writeSession writes session to a config file.
func (a *App) writeSession() error {
	data := []byte(a.session)
	err := writeConfigFile("forge/session", data)
	if err != nil {
		return err
	}
	return nil
}

// removeSession removes sesson config file.
func (a *App) removeSession() error {
	a.user = ""
	a.session = ""
	err := removeConfigFile("forge/session")
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

func (a *App) ReloadUserData() error {
	sec, err := getUserDataSection(a.host, a.session, a.user, "canal")
	if err != nil {
		return err
	}
	err = json.Unmarshal([]byte(sec.Data["options.assigned_only"]), &a.state.Options.AssignedOnly)
	if err != nil {
		// Empty or invalid data. Set the default value.
		a.state.Options.AssignedOnly = false
	}
	a.state.ExposedProperties = make(map[string][]string)
	for key, data := range sec.Data {
		_, entType, found := strings.Cut(key, "exposed_properties.")
		if !found {
			continue
		}
		var props []string
		err = json.Unmarshal([]byte(data), &props)
		if err != nil {
			// Invalid data. Set the default value.
			props = make([]string, 0)
		}
		a.state.ExposedProperties[entType] = props
	}
	return nil
}

func (a *App) ToggleExposeProperty(entType, prop string) error {
	props := a.state.ExposedProperties[entType]
	if props == nil {
		props = make([]string, 0)
	}
	idx := -1
	for i, p := range props {
		if p == prop {
			idx = i
			break
		}
	}
	if idx == -1 {
		props = append(props, prop)
	} else {
		props = append(props[:idx], props[idx+1:]...)
	}
	data, err := json.Marshal(props)
	if err != nil {
		return err
	}
	err = setUserData(a.host, a.session, a.user, "exposed_properties."+entType, string(data))
	if err != nil {
		return err
	}
	a.state.ExposedProperties[entType] = props
	return nil
}

// addRecentPath adds a path to head of recent paths.
// If the path has already in recent paths, it will move to head instead.
func (a *App) addRecentPath(path string) error {
	err := arrangeRecentPaths(a.host, a.session, path, 0)
	if err != nil {
		return err
	}
	paths := make([]string, 0)
	for _, pth := range a.state.RecentPaths {
		if path != pth {
			paths = append(paths, pth)
		}
	}
	a.state.RecentPaths = append([]string{path}, paths...)
	return nil
}

// Logout forgets session info of latest logged in user.
func (a *App) Logout() error {
	a.assigned = nil
	a.state = a.newState()
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
func (a *App) Program(prog string) *Program {
	return a.program[prog]
}

func (a *App) legacyPrograms(programs []string) []string {
	legacy := make([]string, 0)
	for _, prog := range programs {
		if a.program[prog] == nil {
			legacy = append(legacy, prog)
		}
	}
	return legacy
}

// ReloadUserSetting get user setting from host, and remember it.
func (a *App) ReloadUserSetting() error {
	setting, err := getUserSetting(a.host, a.session, a.user)
	if err != nil {
		return err
	}
	a.state.LegacyPrograms = a.legacyPrograms(setting.ProgramsInUse)
	a.state.ProgramsInUse = setting.ProgramsInUse
	a.state.RecentPaths = setting.RecentPaths
	a.entrySorters = a.makeEntrySorters(setting.EntryPageSortProperty)
	return nil
}

// AddProgramInUse adds a in-use program to where user wants.
func (a *App) AddProgramInUse(prog string, at int) error {
	err := arrangeProgramInUse(a.host, a.session, prog, at)
	if err != nil {
		return err
	}
	key := func(s string) string { return s }
	a.state.ProgramsInUse = forge.Arrange(a.state.ProgramsInUse, prog, at, key, false)
	return nil
}

// RemoveProgramInUse removes a in-use program.
func (a *App) RemoveProgramInUse(prog string) error {
	err := arrangeProgramInUse(a.host, a.session, prog, -1)
	if err != nil {
		return err
	}
	key := func(s string) string { return s }
	a.state.ProgramsInUse = forge.Arrange(a.state.ProgramsInUse, prog, -1, key, false)
	return nil
}

// getEnv get value of an environment variable from `env`.
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

// setEnv set value of an environment variable to `env`.
func setEnv(key, val string, env []string) []string {
	idx := -1
	for i := len(env) - 1; i >= 0; i-- {
		e := env[i]
		kv := strings.SplitN(e, "=", -1)
		if len(kv) != 2 {
			continue
		}
		k := strings.TrimSpace(kv[0])
		if k == key {
			idx = i
			break
		}
	}
	if idx < 0 {
		env = append(env, key+"="+val)
		return env
	}
	env[idx] = key + "=" + val
	return env
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
	env := os.Environ()
	forgeEnv, err := entryEnvirons(a.host, a.session, path)
	if err != nil {
		return nil, err
	}
	for _, e := range forgeEnv {
		env = setEnv(e.Name, e.Eval, env)
	}
	for _, e := range a.config.Envs {
		kv := strings.SplitN(e, "=", 2)
		env = setEnv(kv[0], kv[1], env)
	}
	sec, err := getUserDataSection(a.host, a.session, a.user, "environ")
	if err != nil {
		// TODO: shouldn't rely on error messages.
		if err.Error() != "user data section is not exists: environ" {
			return nil, err
		}
	}
	if sec != nil {
		for key, val := range sec.Data {
			env = setEnv(key, val, env)
		}
	}
	return env, nil
}

// NewElement creates a new element by creating a scene file.
func (a *App) NewElement(path, name, prog string) error {
	env, err := a.EntryEnvirons(path)
	if err != nil {
		return err
	}
	sceneDir := getEnv("SCENE_DIR", env)
	if sceneDir == "" {
		return fmt.Errorf("no scene directory information: check SCENE_DIR environ")
	}
	sceneDir = evalEnvString(sceneDir, env)
	err = os.MkdirAll(sceneDir, 0755)
	if err != nil {
		return err
	}
	sceneNameEnv := "SCENE_NAME"
	if name == "" {
		sceneNameEnv = "MAIN_SCENE_NAME"
	}
	sceneName := getEnv(sceneNameEnv, env)
	if sceneName == "" {
		return fmt.Errorf("no scene name information: check " + sceneNameEnv + " environ")
	}
	pg := a.Program(prog)
	if pg == nil {
		return fmt.Errorf("unknown program: %s", prog)
	}
	env = append(env, "ELEM="+name)
	env = append(env, "EXT="+pg.Ext)
	env = append(env, "FORGE_SESSION="+a.session)
	// find lastest version of the element, and increment 1 from it.
	var scene string
	verPre := "v"
	verDigits := "001"
	// override verPre, verDigits if NEW_VER environ defined.
	ver := getEnv("NEW_VER", env)
	if ver != "" {
		for i := len(ver) - 1; i >= 0; i-- {
			_, err := strconv.Atoi(string(ver[i]))
			if err != nil {
				verPre = ver[:i+1]
				verDigits = ver[i+1:]
			}
		}
	}
	nDigits := len(verDigits)
	start, _ := strconv.Atoi(verDigits)
	last, err := a.LastVersionOfElement(path, name, prog)
	if err != nil {
		e := &ElemNotExistError{}
		if !errors.As(err, &e) {
			return err
		}
	}
	if last != "" {
		last = strings.TrimPrefix(last, "v")
		n, err := strconv.Atoi(last)
		if err != nil {
			return err
		}
		start = n + 1
	}
	for n := start; ; n++ {
		v := strconv.Itoa(n)
		z := nDigits - len(v)
		if z < 0 {
			z = 0
		}
		ver := verPre + strings.Repeat("0", z) + v
		env = setEnv("VER", ver, env)
		name := evalEnvString(sceneName, env)
		scene = sceneDir + "/" + name
		_, err := os.Stat(scene)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			// found the first scene path that is not exists.
			break
		}
	}
	if scene == "" {
		return fmt.Errorf("couldn't get appropriate scene name:", sceneName)
	}
	env = append(env, "SCENE="+scene)
	createCmd := make([]string, 0, len(pg.CreateCmd))
	for _, c := range pg.CreateCmd {
		c = evalEnvString(c, env)
		c = strings.TrimSpace(c)
		if c != "" {
			createCmd = append(createCmd, c)
		}
	}
	cmd := exec.Command(createCmd[0], createCmd[1:]...)
	cmd.Dir = sceneDir
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
	Num   int
	Scene string
}

// ListElements returns elements of a part entry each of which holds versions as well.
func (a *App) ListElements(path string) ([]*Elem, error) {
	env, err := a.EntryEnvirons(path)
	if err != nil {
		return nil, err
	}
	sceneDir := getEnv("SCENE_DIR", env)
	if sceneDir == "" {
		return nil, fmt.Errorf("no scene directory information: check SCENE_DIR environ")
	}
	sceneDir = evalEnvString(sceneDir, env)
	sceneName := getEnv("SCENE_NAME_QUERY", env)
	sceneName = evalEnvString(sceneName, env)
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
		extra := string(reName.ExpandString([]byte{}, "$EXTRA", name, idxs))
		if extra != "" {
			continue
		}
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
		if strings.HasPrefix(ver, "v") {
			ver = ver[1:]
		}
		num, err := strconv.Atoi(ver)
		if err != nil {
			num = -1
		}
		v.Num = num
		e.Versions = append(e.Versions, v)
		elem[el+"/"+p.Name] = e
	}
	elems := make([]*Elem, 0, len(elem))
	for _, el := range elem {
		sort.Slice(el.Versions, func(i, j int) bool {
			cmp := el.Versions[i].Num - el.Versions[j].Num
			if cmp != 0 {
				return cmp > 0
			}
			// prefer version having more digits
			return len(el.Versions[i].Name) > len(el.Versions[j].Name)
		})
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
	pg := a.Program(prog)
	if pg == nil {
		return "", fmt.Errorf("unknown program: %s", prog)
	}
	env, err := a.EntryEnvirons(path)
	if err != nil {
		return "", err
	}
	sceneDir := getEnv("SCENE_DIR", env)
	if sceneDir == "" {
		return "", fmt.Errorf("no scene directory information: check SCENE_DIR environ")
	}
	sceneDir = evalEnvString(sceneDir, env)
	sceneNameEnv := "SCENE_NAME"
	if elem == "" {
		sceneNameEnv = "MAIN_SCENE_NAME"
	}
	sceneName := getEnv(sceneNameEnv, env)
	if sceneName == "" {
		return "", fmt.Errorf("no scene name information: check " + sceneNameEnv + " environ")
	}
	env = append(env, "ELEM="+elem)
	env = append(env, `VER=(?P<VER>[vV]\d+)`)
	env = append(env, "EXT="+pg.Ext)
	sceneName = evalEnvString(sceneName, env)
	scene := sceneDir + "/" + sceneName
	scene = evalEnvString(scene, env)
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
	vers := make([]Version, 0)
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
		v := Version{Name: ver}
		if strings.HasPrefix(ver, "v") {
			ver = ver[1:]
		}
		num, err := strconv.Atoi(ver)
		if err != nil {
			num = -1
		}
		v.Num = num
		vers = append(vers, v)
	}
	if len(vers) == 0 {
		return "", &ElemNotExistError{elem: elem}
	}
	sort.Slice(vers, func(i, j int) bool {
		cmp := vers[i].Num - vers[j].Num
		if cmp != 0 {
			return cmp > 0
		}
		// prefer version having more digits
		return len(vers[i].Name) > len(vers[j].Name)
	})
	return vers[0].Name, nil
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
	pg := a.Program(prog)
	if pg == nil {
		return "", fmt.Errorf("unknown program: %s", prog)
	}
	env, err := a.EntryEnvirons(path)
	if err != nil {
		return "", err
	}
	sceneDir := getEnv("SCENE_DIR", env)
	if sceneDir == "" {
		return "", fmt.Errorf("no scene directory information: check SCENE_DIR environ")
	}
	sceneDir = evalEnvString(sceneDir, env)
	sceneNameEnv := "SCENE_NAME"
	if elem == "" {
		sceneNameEnv = "MAIN_SCENE_NAME"
	}
	sceneName := getEnv(sceneNameEnv, env)
	if sceneName == "" {
		return "", fmt.Errorf("no scene name information: check " + sceneNameEnv + " environ")
	}
	env = append(env, "ELEM="+elem)
	env = append(env, "VER="+ver)
	env = append(env, "EXT="+pg.Ext)
	sceneName = evalEnvString(sceneName, env)
	scene := sceneDir + "/" + sceneName
	scene = evalEnvString(scene, env)
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
	pg := a.Program(prog)
	if pg == nil {
		return fmt.Errorf("unknown program: %s", prog)
	}
	env, err := a.EntryEnvirons(path)
	if err != nil {
		return err
	}
	sceneDir := getEnv("SCENE_DIR", env)
	if sceneDir == "" {
		return fmt.Errorf("no scene directory information: check SCENE_DIR environ")
	}
	sceneDir = evalEnvString(sceneDir, env)
	sceneNameEnv := "SCENE_NAME"
	if elem == "" {
		sceneNameEnv = "MAIN_SCENE_NAME"
	}
	sceneName := getEnv(sceneNameEnv, env)
	if sceneName == "" {
		return fmt.Errorf("no scene name information: check " + sceneNameEnv + " environ")
	}
	env = append(env, "ELEM="+elem)
	env = append(env, "VER="+ver)
	env = append(env, "EXT="+pg.Ext)
	env = append(env, "FORGE_SESSION="+a.session)
	sceneName = evalEnvString(sceneName, env)
	scene := sceneDir + "/" + sceneName
	scene = evalEnvString(scene, env)
	env = append(env, "SCENE="+scene)
	openCmd := make([]string, 0, len(pg.OpenCmd))
	for _, c := range pg.OpenCmd {
		c = evalEnvString(c, env)
		c = strings.TrimSpace(c)
		if c != "" {
			openCmd = append(openCmd, c)
		}
	}
	cmd := exec.Command(openCmd[0], openCmd[1:]...)
	cmd.Dir = filepath.Dir(scene)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Start()
	if err != nil {
		fmt.Println(err)
	}
	err = a.addRecentPath(path)
	if err != nil {
		return err
	}
	return nil
}

// Dir returns directory path of an entry.
func (a *App) Dir(path string) (string, error) {
	ent, err := getEntry(a.host, a.session, path)
	if err != nil {
		return "", err
	}
	dirTmpl, ok := a.config.Dir[ent.Type]
	if !ok {
		return "", fmt.Errorf("directory not specified")
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

// openPath opens a path which can be a file, directory, or url.
func openPath(path string) error {
	var open []string
	switch runtime.GOOS {
	case "windows":
		open = []string{"cmd", "/c", "start " + path}
	case "darwin":
		open = []string{"open", path}
	case "linux":
		open = []string{"xdg-open", path}
	default:
		log.Fatalf("unsupported os: %s", runtime.GOOS)
	}

	cmd := exec.Command(open[0], open[1:]...)
	_, err := cmd.CombinedOutput()
	return err
}

// Open opens a directory or run a file.
func (a *App) Open(path string) error {
	_, err := os.Stat(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return openPath(path)
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
	return openPath(dir)
}

// OpenURL opens a url page which shows information about the entry.
func (a *App) OpenURL(path string) error {
	return openPath("https://" + a.host + path)
}

func (a *App) GetClipboardText() (string, error) {
	return wails.ClipboardGetText(a.ctx)
}

func (a *App) Quit() {
	wails.Quit(a.ctx)
}
