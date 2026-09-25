package polyglot

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Bridge routes identification, applet selection, execution, and translation.
type Bridge struct{}

func NewBridge() *Bridge { return &Bridge{} }

// Identify returns language id, confidence, and chosen applet id.
func (b *Bridge) Identify(code string) map[string]string {
	id, conf := DetectLanguage(code)
	applet, reason := ChooseApplet("", code)
	aid := ""
	ready := "false"
	if applet != nil {
		aid = applet.ID
		if AppletReady(applet) {
			ready = "true"
		}
	}
	return map[string]string{
		"language":   id,
		"confidence": conf,
		"applet":     aid,
		"reason":     reason,
		"ready":      ready,
	}
}

// Run selects applet (hint or detect) and executes code.
func (b *Bridge) Run(langHint, code string) Result {
	applet, reason := ChooseApplet(langHint, code)
	if applet == nil {
		return Result{Err: fmt.Errorf("no applet for code (%s)", reason)}
	}
	if applet.ID == "nvs" {
		return Result{Output: code, Err: nil}
	}
	if !AppletReady(applet) {
		return Result{Err: fmt.Errorf("applet %s not ready (missing toolchain)", applet.ID)}
	}
	if applet.Run == nil {
		return Result{Err: fmt.Errorf("applet %s cannot run", applet.ID)}
	}
	r := applet.Run(code)
	if r.Err == nil {
		// annotate lightly for debugging interop chain
		_ = reason
	}
	return r
}

// Translate code between languages (via NvS intermediate when needed).
func (b *Bridge) Translate(from, to, code string) (string, error) {
	return Translate(from, to, code)
}

// AssembleNvS builds NvS source from a native snippet using detection + ToNvS.
func (b *Bridge) AssembleNvS(langHint, code string) (string, error) {
	applet, _ := ChooseApplet(langHint, code)
	if applet == nil {
		return "", fmt.Errorf("could not choose applet")
	}
	if applet.ID == "nvs" {
		return code, nil
	}
	if applet.ToNvS == nil {
		return "", fmt.Errorf("applet %s cannot assemble NvS", applet.ID)
	}
	return applet.ToNvS(code)
}

// ReplicateNative builds native source from NvS using FromNvS.
func (b *Bridge) ReplicateNative(lang, nvsCode string) (string, error) {
	applet, ok := GetApplet(lang)
	if !ok {
		return "", fmt.Errorf("unknown language %s", lang)
	}
	if applet.FromNvS == nil {
		return "", fmt.Errorf("applet %s cannot replicate from NvS", applet.ID)
	}
	return applet.FromNvS(nvsCode)
}

// RoundTrip: native → NvS → native (or NvS → native → run).
func (b *Bridge) RoundTrip(lang, code string) (nvsOut, nativeOut, runOut string, err error) {
	nvsOut, err = b.AssembleNvS(lang, code)
	if err != nil {
		return
	}
	nativeOut, err = b.ReplicateNative(lang, nvsOut)
	if err != nil {
		return
	}
	r := b.Run(lang, nativeOut)
	runOut = r.Output
	err = r.Err
	return
}

// CatalogJSON lists applets and readiness for NvS consumption.
func CatalogJSON() string {
	type row struct {
		ID           string   `json:"id"`
		Name         string   `json:"name"`
		Ready        bool     `json:"ready"`
		Extensions   []string `json:"extensions"`
		Capabilities []string `json:"capabilities"`
		Aliases      []string `json:"aliases"`
	}
	var rows []row
	for _, a := range ListApplets() {
		rows = append(rows, row{
			ID: a.ID, Name: a.Name, Ready: AppletReady(a),
			Extensions: a.Extensions, Capabilities: a.Capabilities, Aliases: a.Aliases,
		})
	}
	b, _ := json.MarshalIndent(rows, "", "  ")
	return string(b)
}

// DescribeApplet returns a human-readable summary.
func DescribeApplet(id string) string {
	a, ok := GetApplet(id)
	if !ok {
		return "unknown applet: " + id
	}
	return fmt.Sprintf("%s (%s) ready=%v caps=[%s] ext=[%s]",
		a.Name, a.ID, AppletReady(a),
		strings.Join(a.Capabilities, ","),
		strings.Join(a.Extensions, ","),
	)
}
