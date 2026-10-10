// cs2-demo: pick a downloaded CS2 demo in a web UI and play it in the game.
package main

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/nwaples/rardecode/v2"
)

//go:embed index.html
var page []byte

var version = "dev" // set by release.sh with -ldflags "-X main.version=..."

//go:embed logo.svg
var logo []byte

const outDir = "dl" // demos are extracted to <csgo>/dl: a short, plain-ASCII path CS2 can always play

var (
	demoDir  = flag.String("demos", filepath.Join(os.Getenv("USERPROFILE"), "Downloads"), "folder with downloaded demos")
	steamDir = flag.String("steam", "", "Steam install folder (default: read from the registry)")
	port     = flag.Int("port", 7350, "web UI port")
	csgoDir  string
)

func main() {
	flag.Parse()
	page = bytes.ReplaceAll(page, []byte("{{version}}"), []byte(version))
	page = bytes.ReplaceAll(page, []byte("{{folder}}"), []byte(html.EscapeString(*demoDir)))
	if *steamDir == "" {
		*steamDir = findSteam()
	}
	var err error
	if csgoDir, err = findCSGO(*steamDir); err != nil {
		fatal(err)
	}
	log.Printf("demos: %s\ncs2:   %s", *demoDir, csgoDir)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.Write(page) })
	http.HandleFunc("/logo.svg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Write(logo)
	})
	http.HandleFunc("/api/demos", handle(func(r *http.Request) (any, error) { return listDemos(*demoDir) }))
	http.HandleFunc("/api/files", handle(func(r *http.Request) (any, error) {
		return listFiles(filepath.Join(*demoDir, filepath.Base(r.URL.Query().Get("demo"))))
	}))
	http.HandleFunc("/api/play", handle(func(r *http.Request) (any, error) {
		// JSON content type forces a CORS preflight, so other websites can't trigger this.
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			return nil, errors.New("bad request")
		}
		var req struct {
			Demo, File string
			Restart    bool
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return nil, err
		}
		return play(filepath.Join(*demoDir, filepath.Base(req.Demo)), req.File, req.Restart)
	}))
	http.HandleFunc("/api/log", handle(func(r *http.Request) (any, error) { return gameLog(), nil }))

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	ln, err := net.Listen("tcp", addr)
	openBrowser := func() { openURL("http://" + addr) }
	if err != nil {
		// Probably already running: just show the open one.
		openBrowser()
		return
	}
	log.Printf("open http://%s (close this window to stop)", addr)
	openBrowser()
	fatal(http.Serve(ln, nil))
}

// fatal keeps the window open so a double-clicked exe shows the error.
func fatal(err error) {
	log.Print(err)
	fmt.Print("Press Enter to exit")
	fmt.Scanln()
	os.Exit(1)
}

// openURL opens url in Chrome when it is installed, else in the default browser.
// Windows kept opening Edge for a Chrome user, through both "start" and the registry.
func openURL(url string) {
	for _, dir := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LocalAppData")} {
		chrome := filepath.Join(dir, "Google", "Chrome", "Application", "chrome.exe")
		if _, err := os.Stat(chrome); err == nil && dir != "" && exec.Command(chrome, url).Start() == nil {
			return
		}
	}
	exec.Command("cmd", "/c", "start", "", url).Start()
}

// findSteam reads the Steam folder from the registry, falling back to the default path.
func findSteam() string {
	out, _ := exec.Command("reg", "query", `HKCU\Software\Valve\Steam`, "/v", "SteamPath").Output()
	if m := regexp.MustCompile(`SteamPath\s+REG_SZ\s+(.+)`).FindStringSubmatch(string(out)); m != nil {
		return filepath.FromSlash(strings.TrimSpace(m[1]))
	}
	return `C:\Program Files (x86)\Steam`
}

func handle(f func(*http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, err := f(r)
		if errors.Is(err, errRunning) {
			http.Error(w, err.Error(), http.StatusConflict) // the UI offers a restart button
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(v)
	}
}

// findCSGO looks for the CS2 game/csgo folder in every Steam library.
func findCSGO(steam string) (string, error) {
	libs := []string{steam}
	if vdf, err := os.ReadFile(filepath.Join(steam, "steamapps", "libraryfolders.vdf")); err == nil {
		for _, m := range regexp.MustCompile(`"path"\s+"([^"]+)"`).FindAllStringSubmatch(string(vdf), -1) {
			libs = append(libs, strings.ReplaceAll(m[1], `\\`, `\`))
		}
	}
	for _, lib := range libs {
		dir := filepath.Join(lib, "steamapps", "common", "Counter-Strike Global Offensive", "game", "csgo")
		if _, err := os.Stat(dir); err == nil {
			return dir, nil
		}
	}
	return "", fmt.Errorf("CS2 not found in Steam libraries %v (use -steam)", libs)
}

type demo struct {
	Name string    `json:"name"`
	Size int64     `json:"size"`
	Mod  time.Time `json:"mod"`
}

func listDemos(dir string) ([]demo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	demos := []demo{}
	for _, e := range entries {
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".dem", ".zip", ".rar":
			if info, err := e.Info(); err == nil && !e.IsDir() {
				demos = append(demos, demo{e.Name(), info.Size(), info.ModTime()})
			}
		}
	}
	sort.Slice(demos, func(i, j int) bool { return demos[i].Mod.After(demos[j].Mod) })
	return demos, nil
}

// eachDem calls f for every .dem file in path (a .dem, .zip or .rar).
// f returns true to stop.
func eachDem(path string, f func(name string, r io.Reader) bool) error {
	isDem := func(n string) bool { return strings.EqualFold(filepath.Ext(n), ".dem") }
	switch strings.ToLower(filepath.Ext(path)) {
	case ".dem":
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		f(filepath.Base(path), file)
	case ".zip":
		z, err := zip.OpenReader(path)
		if err != nil {
			return err
		}
		defer z.Close()
		for _, e := range z.File {
			if !isDem(e.Name) {
				continue
			}
			r, err := e.Open()
			if err != nil {
				return err
			}
			stop := f(e.Name, r)
			r.Close()
			if stop {
				return nil
			}
		}
	case ".rar":
		rr, err := rardecode.OpenReader(path)
		if err != nil {
			return err
		}
		defer rr.Close()
		for {
			h, err := rr.Next()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			if isDem(h.Name) && f(h.Name, rr) {
				return nil
			}
		}
	default:
		return errors.New("not a demo: " + path)
	}
	return nil
}

func listFiles(path string) ([]string, error) {
	names := []string{}
	err := eachDem(path, func(name string, _ io.Reader) bool { names = append(names, name); return false })
	return names, err
}

var unsafeChars = regexp.MustCompile(`[^a-z0-9]+`)

// shortName turns "Vitality vs MOUZ - m1 - Mirage.dem" into "vitality_vs_mouz_m1_mirage",
// safe to type into the console without quotes.
func shortName(file string) string {
	base := strings.TrimSuffix(filepath.Base(filepath.ToSlash(file)), filepath.Ext(file))
	return strings.Trim(unsafeChars.ReplaceAllString(strings.ToLower(base), "_"), "_")
}

// CS2 only opens a console port (-netconport) in Workshop Tools mode, so a running game can't
// be told to play a demo. Like CS Demo Manager without its plugin, we restart CS2 with the demo.
var errRunning = errors.New("CS2 is open. To load a demo, CS2 has to restart. Leave any match first")

func play(path, file string, restart bool) (string, error) {
	name := shortName(filepath.Base(path)) + "_" + shortName(file)
	dst := filepath.Join(csgoDir, outDir, name+".dem")
	if _, err := os.Stat(dst); err != nil {
		if err := extract(path, file, dst); err != nil {
			return "", err
		}
	}

	if running("cs2.exe") {
		if !restart {
			return "", errRunning
		}
		exec.Command("taskkill", "/F", "/IM", "cs2.exe").Run()
		for i := 0; running("cs2.exe"); i++ {
			if i == 30 {
				return "", errors.New("could not close CS2")
			}
			time.Sleep(500 * time.Millisecond)
		}
		time.Sleep(2 * time.Second) // let the old process release its files
	}
	if !running("steam.exe") {
		return "", errors.New("Steam is not running. Start Steam, then click Play again")
	}
	// Start cs2.exe directly, like CS Demo Manager does: "steam.exe -applaunch" drops the options.
	if info, err := os.Stat(filepath.Join(csgoDir, "console.log")); err == nil {
		logOffset = info.Size()
	}
	exe := filepath.Join(csgoDir, "..", "bin", "win64", "cs2.exe")
	// -insecure matches CS Demo Manager: CS2 queued +playdemo without it and never ran it.
	// The demo path is relative to game/csgo, so it has no spaces: CS2 fails on some full paths.
	// -condebug writes game/csgo/console.log, which the page shows to explain failures.
	if err := exec.Command(exe, "-insecure", "-novid", "-condebug", "+playdemo", outDir+"/"+name+".dem").Start(); err != nil {
		return "", err
	}
	return "Starting CS2 with " + name + ".\nCS2 runs in insecure mode, for demos only. Restart it from Steam before you play matches.", nil
}

var logOffset int64 // console.log size before the last launch

// gameLog returns the console lines CS2 wrote since the last launch, from the
// first playdemo line on, so errors without the word "demo" show up too.
func gameLog() []string {
	b, _ := os.ReadFile(filepath.Join(csgoDir, "console.log"))
	if int64(len(b)) >= logOffset {
		b = b[logOffset:]
	}
	text := string(b)
	i := strings.Index(text, "playdemo")
	if i < 0 {
		return []string{}
	}
	lines := strings.Split(strings.TrimSpace(text[strings.LastIndex(text[:i], "\n")+1:]), "\n")
	if len(lines) > 40 {
		lines = lines[len(lines)-40:]
	}
	return lines
}

func running(image string) bool {
	out, _ := exec.Command("tasklist", "/FI", "IMAGENAME eq "+image, "/NH").Output()
	return strings.Contains(strings.ToLower(string(out)), image)
}

func extract(path, file, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	found := false
	var copyErr error
	err = eachDem(path, func(name string, r io.Reader) bool {
		if name != file {
			return false
		}
		found = true
		_, copyErr = io.Copy(out, r)
		return true
	})
	out.Close()
	if err == nil {
		err = copyErr
	}
	if err == nil && !found {
		err = errors.New("demo file not found: " + file)
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}
