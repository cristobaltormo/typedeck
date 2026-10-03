package obs

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Password string `json:"password"`
}

var Commands = []string{"scene", "scene_next", "scene_prev", "stream", "stream_start", "stream_stop", "record", "record_pause", "mute", "replay_save", "virtualcam", "studio", "studio_transition"}

func NeedsTarget(cmd string) bool { return cmd == "scene" || cmd == "mute" }

type msg struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d"`
}

type session struct{ ws *wsConn }

func connect(cfg Config) (*session, error) {
	ws, err := wsDial(hostPort(cfg.Host, cfg.Port), "obswebsocket.json", 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("could not connect to OBS (%v). Turn on the WebSocket server in Tools, WebSocket Server Settings", simplify(err))
	}
	s := &session{ws}
	raw, err := ws.ReadText()
	if err != nil {
		ws.Close()
		return nil, err
	}
	var hello struct {
		D struct {
			Auth *struct{ Challenge, Salt string } `json:"authentication"`
		} `json:"d"`
	}
	if err := json.Unmarshal(raw, &hello); err != nil {
		ws.Close()
		return nil, errors.New("invalid answer from OBS")
	}
	id := map[string]any{"rpcVersion": 1}
	if a := hello.D.Auth; a != nil {
		if cfg.Password == "" {
			ws.Close()
			return nil, errors.New("OBS asks for a password: enter it in Settings")
		}
		sec := sha256.Sum256([]byte(cfg.Password + a.Salt))
		secB := base64.StdEncoding.EncodeToString(sec[:])
		auth := sha256.Sum256([]byte(secB + a.Challenge))
		id["authentication"] = base64.StdEncoding.EncodeToString(auth[:])
	}
	if err := s.send(1, id); err != nil {
		ws.Close()
		return nil, err
	}
	for {
		raw, err = ws.ReadText()
		if err != nil {
			ws.Close()
			if strings.Contains(err.Error(), "EOF") {
				return nil, errors.New("OBS rejected the password")
			}
			return nil, err
		}
		var m msg
		if json.Unmarshal(raw, &m) == nil && m.Op == 2 {
			return s, nil
		}
	}
}

func simplify(err error) string {
	s := err.Error()
	if i := strings.LastIndex(s, ": "); i >= 0 {
		s = s[i+2:]
	}
	return s
}

func (s *session) send(op int, d any) error {
	b, _ := json.Marshal(map[string]any{"op": op, "d": d})
	s.ws.deadline(5 * time.Second)
	return s.ws.WriteText(b)
}

func (s *session) call(typ string, data any) (map[string]any, error) {
	req := map[string]any{"requestType": typ, "requestId": typ}
	if data != nil {
		req["requestData"] = data
	}
	if err := s.send(6, req); err != nil {
		return nil, err
	}
	for {
		s.ws.deadline(5 * time.Second)
		raw, err := s.ws.ReadText()
		if err != nil {
			return nil, err
		}
		var m struct {
			Op int `json:"op"`
			D  struct {
				RequestID     string `json:"requestId"`
				RequestStatus struct {
					Result  bool   `json:"result"`
					Code    int    `json:"code"`
					Comment string `json:"comment"`
				} `json:"requestStatus"`
				ResponseData map[string]any `json:"responseData"`
			} `json:"d"`
		}
		if json.Unmarshal(raw, &m) != nil || m.Op != 7 || m.D.RequestID != typ {
			continue
		}
		if !m.D.RequestStatus.Result {
			return nil, &reqError{Code: m.D.RequestStatus.Code, Comment: m.D.RequestStatus.Comment}
		}
		return m.D.ResponseData, nil
	}
}

type reqError struct {
	Code    int
	Comment string
}

func (e *reqError) Error() string {
	if e.Comment != "" {
		return e.Comment
	}
	return fmt.Sprintf("OBS could not do it (code %d)", e.Code)
}

func onOff(v any, on, off string) string {
	if b, _ := v.(bool); b {
		return on
	}
	return off
}

func Run(cfg Config, cmd, target string) (string, error) {
	s, err := connect(cfg)
	if err != nil {
		return "", err
	}
	defer s.ws.Close()
	switch cmd {
	case "scene":
		target = strings.TrimSpace(target)
		if target == "" {
			return "", errors.New("the scene name is missing")
		}
		if n, ok := sceneNumber(target); ok {
			names, _, err := s.scenes()
			if err != nil {
				return "", err
			}
			if n < 1 || n > len(names) {
				return "", fmt.Errorf("OBS only has %d scenes", len(names))
			}
			target = names[n-1]
		}
		_, err := s.call("SetCurrentProgramScene", map[string]any{"sceneName": target})
		return target, err
	case "scene_next", "scene_prev":
		return s.stepScene(cmd == "scene_next")
	case "stream":
		r, err := s.call("ToggleStream", nil)
		return onOff(r["outputActive"], "live", "stream stopped"), err
	case "stream_start":
		_, err := s.call("StartStream", nil)
		return "live", ignoreActive(err)
	case "stream_stop":
		_, err := s.call("StopStream", nil)
		return "stream stopped", ignoreActive(err)
	case "record":
		r, err := s.call("ToggleRecord", nil)
		return onOff(r["outputActive"], "recording", "recording stopped"), err
	case "record_pause":
		_, err := s.call("ToggleRecordPause", nil)
		return "", err
	case "mute":
		name, err := s.inputName(target)
		if err != nil {
			return "", err
		}
		target = name
		r, err := s.call("ToggleInputMute", map[string]any{"inputName": target})
		return target + ": " + onOff(r["inputMuted"], "muted", "unmuted"), err
	case "replay_save":
		_, err := s.call("SaveReplayBuffer", nil)
		return "replay saved", err
	case "virtualcam":
		r, err := s.call("ToggleVirtualCam", nil)
		return onOff(r["outputActive"], "virtual camera on", "virtual camera off"), err
	case "studio":
		r, err := s.call("GetStudioModeEnabled", nil)
		if err != nil {
			return "", err
		}
		on, _ := r["studioModeEnabled"].(bool)
		_, err = s.call("SetStudioModeEnabled", map[string]any{"studioModeEnabled": !on})
		return onOff(!on, "studio mode on", "studio mode off"), err
	case "studio_transition":
		_, err := s.call("TriggerStudioModeTransition", nil)
		return "", err
	}
	return "", fmt.Errorf("unknown OBS command: %s", cmd)
}

func Active(cfg Config, what string) (bool, error) {
	s, err := connect(cfg)
	if err != nil {
		return false, err
	}
	defer s.ws.Close()
	req := "GetStreamStatus"
	if what == "record" {
		req = "GetRecordStatus"
	}
	r, err := s.call(req, nil)
	if err != nil {
		return false, err
	}
	on, _ := r["outputActive"].(bool)
	return on, nil
}

func ignoreActive(err error) error {
	var re *reqError
	if errors.As(err, &re) && (re.Code == 500 || re.Code == 501) {
		return nil
	}
	return err
}

func sceneNumber(t string) (int, bool) {
	if !strings.HasPrefix(t, "#") {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(t, "#"))
	return n, err == nil
}

func (s *session) scenes() ([]string, string, error) {
	r, err := s.call("GetSceneList", nil)
	if err != nil {
		return nil, "", err
	}
	list, _ := r["scenes"].([]any)
	cur, _ := r["currentProgramSceneName"].(string)
	if len(list) == 0 {
		return nil, cur, errors.New("OBS has no scenes")
	}
	names := make([]string, len(list))
	for i, it := range list {
		m, _ := it.(map[string]any)
		names[len(list)-1-i], _ = m["sceneName"].(string)
	}
	return names, cur, nil
}

func (s *session) inputName(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target != "" && !strings.HasPrefix(target, "@") {
		return target, nil
	}
	key := map[string]string{"": "mic1", "@mic": "mic1", "@desktop": "desktop1"}[target]
	if key == "" {
		return "", fmt.Errorf("unknown source: %s", target)
	}
	r, err := s.call("GetSpecialInputs", nil)
	if err != nil {
		return "", err
	}
	if n, _ := r[key].(string); n != "" {
		return n, nil
	}
	return "", errors.New("OBS has no such audio source configured")
}

func (s *session) stepScene(next bool) (string, error) {
	names, cur, err := s.scenes()
	if err != nil {
		return "", err
	}
	idx := 0
	for i, n := range names {
		if n == cur {
			idx = i
		}
	}
	if next {
		idx = (idx + 1) % len(names)
	} else {
		idx = (idx - 1 + len(names)) % len(names)
	}
	_, err = s.call("SetCurrentProgramScene", map[string]any{"sceneName": names[idx]})
	return names[idx], err
}

type Info struct {
	Version string   `json:"version"`
	Scenes  []string `json:"scenes"`
	Inputs  []string `json:"inputs"`
}

func Probe(cfg Config) (*Info, error) {
	s, err := connect(cfg)
	if err != nil {
		return nil, err
	}
	defer s.ws.Close()
	info := &Info{}
	if r, err := s.call("GetVersion", nil); err == nil {
		info.Version, _ = r["obsVersion"].(string)
	}
	if r, err := s.call("GetSceneList", nil); err == nil {
		list, _ := r["scenes"].([]any)
		for i := len(list) - 1; i >= 0; i-- {
			if m, ok := list[i].(map[string]any); ok {
				if n, _ := m["sceneName"].(string); n != "" {
					info.Scenes = append(info.Scenes, n)
				}
			}
		}
	}
	if r, err := s.call("GetInputList", nil); err == nil {
		list, _ := r["inputs"].([]any)
		for _, it := range list {
			if m, ok := it.(map[string]any); ok {
				k, _ := m["inputKind"].(string)
				if n, _ := m["inputName"].(string); n != "" && (strings.Contains(k, "audio") || strings.Contains(k, "wasapi") || strings.Contains(k, "pulse") || strings.Contains(k, "coreaudio") || strings.Contains(k, "alsa")) {
					info.Inputs = append(info.Inputs, n)
				}
			}
		}
	}
	return info, nil
}
