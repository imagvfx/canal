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
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/imagvfx/forge"
)

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
	openCmd       string
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
	a.state = a.newState()
	err := a.ReloadBase(true)
	if err != nil {
		return fmt.Errorf("reload base: %v", err)
	}
	path := "/"
	if len(a.state.RecentPaths) != 0 {
		path = a.state.RecentPaths[0]
	}
	err = a.GoTo(path) // at least one page needed in history
	if err != nil {
		return err
	}
	return nil
}

func (a *App) ReloadEntry() error {
	err := a.ReloadBase(false)
	if err != nil {
		return err
	}
	path := a.state.Path
	a.state.Entry, err = a.GetEntry(path)
	if err != nil {
		return err
	}
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
	dir, err := a.Dir(path)
	if err != nil {
		return err
	}
	a.state.Dir = dir
	dirExists, err := a.DirExists(dir)
	if err != nil {
		return err
	}
	a.state.DirExists = dirExists
	return nil
}

// OpenLoginPage shows login page to user.
func (a *App) OpenLoginPage(key string) error {
	cmd := exec.Command(a.openCmd, "https://"+a.host+"/login?app_session_key="+key)
	_, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	return nil
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
	data, err := readConfigFile("session")
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
	a.user = toks[0]
	a.session = toks[1]
	return nil
}

// writeSession writes session to a config file.
func (a *App) writeSession() error {
	data := []byte(a.user + " " + a.session)
	err := writeConfigFile("session", data)
	if err != nil {
		return err
	}
	return nil
}

// removeSession removes sesson config file.
func (a *App) removeSession() error {
	a.user = ""
	a.session = ""
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

func (a *App) ReloadUserData() error {
	sec, err := getUserDataSection(a.host, a.session, a.user)
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
	forgeEnv, err := entryEnvirons(a.host, a.session, path)
	if err != nil {
		return nil, err
	}
	env = os.Environ()
	for _, e := range a.config.Envs {
		env = append(env, e)
	}
	for _, e := range forgeEnv {
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
	pg := a.Program(prog)
	if pg == nil {
		return fmt.Errorf("unknown program: %s", prog)
	}
	env = append(env, "ELEM="+name)
	env = append(env, "VER="+getEnv("NEW_VER", env))
	env = append(env, "EXT="+pg.Ext)
	env = append(env, "FORGE_SESSION="+a.session)
	scene := evalEnvString(a.config.Scene, env)
	sceneDir := filepath.Dir(scene)
	err = os.MkdirAll(sceneDir, 0755)
	if err != nil {
		return err
	}
	env = append(env, "SCENE="+scene)
	createCmd := append([]string{}, pg.CreateCmd...)
	for i, c := range createCmd {
		createCmd[i] = evalEnvString(c, env)
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
	pg := a.Program(prog)
	if pg == nil {
		return "", fmt.Errorf("unknown program: %s", prog)
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
	pg := a.Program(prog)
	if pg == nil {
		return "", fmt.Errorf("unknown program: %s", prog)
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
	pg := a.Program(prog)
	if pg == nil {
		return fmt.Errorf("unknown program: %s", prog)
	}
	env, err := a.EntryEnvirons(path)
	if err != nil {
		return err
	}
	env = append(env, "ELEM="+elem)
	env = append(env, "VER="+ver)
	env = append(env, "EXT="+pg.Ext)
	env = append(env, "FORGE_SESSION="+a.session)
	scene := evalEnvString(a.config.Scene, env)
	env = append(env, "SCENE="+scene)
	openCmd := append([]string{}, pg.OpenCmd...)
	for i, c := range openCmd {
		openCmd[i] = evalEnvString(c, env)
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

// Open opens a directory or run a file.
func (a *App) Open(ent string) error {
	_, err := os.Stat(ent)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	cmd := exec.Command(a.openCmd, ent)
	_, err = cmd.CombinedOutput()
	if err != nil {
		return err
	}
	return nil
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
	cmd := exec.Command(a.openCmd, "https://"+a.host+path)
	_, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	return nil
}
