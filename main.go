// cs2-demo: pick a downloaded CS2 demo in a web UI and play it in the game.
package main

import (
	"archive/zip"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
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

const (
	netconPort = "2121"
	outDir     = "dl" // demos are extracted to <csgo>/dl so playdemo gets a short path
)

var (
	demoDir  = flag.String("demos", filepath.Join(os.Getenv("USERPROFILE"), "Downloads"), "folder with downloaded demos")
	steamDir = flag.String("steam", `C:\Program Files (x86)\Steam`, "Steam install folder")
	port     = flag.Int("port", 7350, "web UI port")
	csgoDir  string
)

func main() {
	flag.Parse()
	var err error
	if csgoDir, err = findCSGO(*steamDir); err != nil {
		log.Fatal(err)
	}
	log.Printf("demos: %s\ncs2:   %s", *demoDir, csgoDir)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.Write(page) })
	http.HandleFunc("/api/demos", handle(func(r *http.Request) (any, error) { return listDemos(*demoDir) }))
	http.HandleFunc("/api/files", handle(func(r *http.Request) (any, error) {
		return listFiles(filepath.Join(*demoDir, filepath.Base(r.URL.Query().Get("demo"))))
	}))
	http.HandleFunc("/api/play", handle(func(r *http.Request) (any, error) {
		// JSON content type forces a CORS preflight, so other websites can't trigger this.
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			return nil, errors.New("bad request")
		}
		var req struct{ Demo, File string }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return nil, err
		}
		return play(filepath.Join(*demoDir, filepath.Base(req.Demo)), req.File)
	}))

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	log.Printf("open http://%s", addr)
	exec.Command("rundll32", "url.dll,FileProtocolHandler", "http://"+addr).Start()
	log.Fatal(http.ListenAndServe(addr, nil))
}

func handle(f func(*http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, err := f(r)
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

func play(path, file string) (string, error) {
	name := shortName(filepath.Base(path)) + "_" + shortName(file)
	dst := filepath.Join(csgoDir, outDir, name+".dem")
	if _, err := os.Stat(dst); err != nil {
		if err := extract(path, file, dst); err != nil {
			return "", err
		}
	}
	cmd := "playdemo " + outDir + "/" + name

	// Game running with -netconport: send the command over its console port.
	if conn, err := net.DialTimeout("tcp", "127.0.0.1:"+netconPort, time.Second); err == nil {
		defer conn.Close()
		_, err = conn.Write([]byte(cmd + "\n"))
		return "sent to running game: " + cmd, err
	}
	if out, _ := exec.Command("tasklist", "/FI", "IMAGENAME eq cs2.exe", "/NH").Output(); strings.Contains(string(out), "cs2.exe") {
		return "", errors.New("CS2 is running without the console port. Add \"-netconport " + netconPort +
			"\" to CS2 launch options in Steam and restart the game once")
	}
	steam := filepath.Join(*steamDir, "steam.exe")
	if err := exec.Command(steam, "-applaunch", "730", "-netconport", netconPort, "+"+cmd).Start(); err != nil {
		return "", err
	}
	return "starting CS2: " + cmd, nil
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
