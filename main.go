package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/charmbracelet/log"
	"github.com/pterm/pterm"
)

type Config struct {
	Debug        bool   `env:"DEBUG"                          envDefault:"false"`
	ClientID     string `env:"SPOTIFY_CLIENT_ID,required"`
	ClientSecret string `env:"SPOTIFY_CLIENT_SECRET,required"`
	LocalServer  string `env:"SPOTIFY_LOCAL_SERVER"           envDefault:"127.0.0.1:4001"`
	RedirectURI  string `env:"SPOTIFY_REDIRECT_URI"           envDefault:"http://127.0.0.1:4001/callback"`
	Scope        string `env:"SPOTIFY_SCOPE"                  envDefault:"user-library-read user-library-modify"`
}

// LogLevel returns the log level based on the debug flag.
func (c *Config) LogLevel() log.Level {
	if c.Debug {
		return log.DebugLevel
	}
	return log.InfoLevel
}

var cfg Config

func init() {
	if err := env.Parse(&cfg); err != nil {
		log.Fatal("failed to parse environment variables", "error", err)
	}
}

func main() {
	logger := log.NewWithOptions(os.Stderr, log.Options{
		ReportCaller: true,
		// ReportTimestamp: true,
		TimeFormat: time.Kitchen,
		// Prefix:          "spotify utils 🎵 ",
		Formatter: log.TextFormatter,
		Level:     cfg.LogLevel(),
	})

	state := generateState(logger)
	authCode := authorize(logger, state)
	accessToken := getAccessToken(logger, authCode)

	albums := fetchSavedAlbums(logger, accessToken)
	for _, album := range albums {
		albumName := album["album"].(map[string]interface{})["name"].(string)
		logger.Debug("album", "album name", albumName)
	}

	confirmAndRemoveAlbums(logger, accessToken, albums)
}

func generateState(logger *log.Logger) string {
	state, err := exec.Command("openssl", "rand", "-hex", "16").Output()
	if err != nil {
		logger.Fatal("failed to generate state", "error", err)
	}
	return strings.TrimSpace(string(state))
}

func authorize(logger *log.Logger, state string) string {
	logger.Info("starting local server for oauth callback")
	authCodeChan := make(chan string)
	go startHTTPServer(logger, authCodeChan)

	authURL := fmt.Sprintf(
		"https://accounts.spotify.com/authorize?response_type=code&client_id=%s&scope=%s&redirect_uri=%s&state=%s",
		cfg.ClientID,
		url.QueryEscape(cfg.Scope),
		url.QueryEscape(cfg.RedirectURI),
		state,
	)

	logger.Info("opening browser for spotify authorization")
	exec.Command("open", authURL).Start()

	logger.Info("waiting for oauth callback")
	authCode := <-authCodeChan
	logger.Info("oauth callback received")
	return authCode
}

func startHTTPServer(logger *log.Logger, authCodeChan chan string) {
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code != "" {
			fmt.Fprintf(w, "authorization code received. you can close this window.")
			authCodeChan <- code
		} else {
			http.Error(w, "authorization failed", http.StatusUnauthorized)
		}
	})
	logger.Fatal("failed to start http server", "error", http.ListenAndServe(cfg.LocalServer, nil))
}

func getAccessToken(logger *log.Logger, authCode string) string {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", authCode)
	data.Set("redirect_uri", cfg.RedirectURI)

	req, err := http.NewRequest("POST", "https://accounts.spotify.com/api/token", strings.NewReader(data.Encode()))
	if err != nil {
		logger.Fatal("failed to create request", "error", err)
	}
	req.SetBasicAuth(cfg.ClientID, cfg.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Fatal("failed to get access token", "error", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Fatal("failed to read response body", "error", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		logger.Fatal("failed to unmarshal response", "error", err)
	}

	accessToken, ok := result["access_token"].(string)
	if !ok {
		logger.Fatal("access token not found in response")
	}

	return accessToken
}

func fetchSavedAlbums(logger *log.Logger, accessToken string) []map[string]interface{} {
	var allItems []map[string]interface{}
	offset := 0
	limit := 50

	for {
		url := fmt.Sprintf("https://api.spotify.com/v1/me/albums?limit=%d&offset=%d", limit, offset)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			logger.Fatal("failed to create request", "error", err)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			logger.Fatal("failed to fetch saved albums", "error", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			logger.Fatal("failed to read response body", "error", err)
		}

		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			logger.Fatal("failed to unmarshal response", "error", err)
		}

		items, ok := result["items"].([]interface{})
		if !ok || len(items) == 0 {
			break
		}

		for _, item := range items {
			allItems = append(allItems, item.(map[string]interface{}))
		}

		total, ok := result["total"].(float64)
		if !ok || offset+limit >= int(total) {
			break
		}

		offset += limit
	}

	return allItems
}

func removeAlbums(logger *log.Logger, accessToken string, albumIDs []string) {
	url := fmt.Sprintf("https://api.spotify.com/v1/me/albums?ids=%s", strings.Join(albumIDs, ","))
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		logger.Fatal("failed to create request", "error", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Fatal("failed to remove albums", "error", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Fatal("failed to remove albums", "status_code", resp.StatusCode)
	}
}

func confirmAndRemoveAlbums(logger *log.Logger, accessToken string, albums []map[string]interface{}) {
	albumNames := make([]string, len(albums))
	albumIDs := make([]string, len(albums))
	for i, album := range albums {
		albumNames[i] = album["album"].(map[string]interface{})["name"].(string)
		albumIDs[i] = album["album"].(map[string]interface{})["id"].(string)
	}

	var confirmed bool
	result, _ := pterm.DefaultInteractiveConfirm.Show()
	pterm.Println()
	pterm.Info.Printfln("You answered: %s", boolToText(result))
	confirmed = result

	if !confirmed {
		logger.Info("operation cancelled by user")
		return
	}
	p, _ := pterm.DefaultProgressbar.WithTotal(len(albumIDs)).WithTitle("Removing albums").Start()

	albumsPerBatch := 20
	totalAlbums := len(albumIDs)

	for i := 0; i < totalAlbums; i += albumsPerBatch {
		end := i + albumsPerBatch
		if end > totalAlbums {
			end = totalAlbums
		}

		removeAlbums(logger, accessToken, albumIDs[i:end])

		// Update progress bar
		p.UpdateTitle(fmt.Sprintf("Removing albums %d/%d", end, totalAlbums))
		p.Add(albumsPerBatch)

		// Debug log for current fetch number and offset
		logger.Debug("removing albums", "current fetch number", i/albumsPerBatch+1, "current offset", i)
	}

	logger.Info("all saved albums removed successfully")
}

// boolToText converts a boolean value to a colored text.
// If the value is true, it returns a green "Yes".
// If the value is false, it returns a red "No".
func boolToText(b bool) string {
	if b {
		return pterm.Green("Yes")
	}
	return pterm.Red("No")
}
