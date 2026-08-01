package pow

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
)

type Challenge struct {
	Algorithm  string  `json:"algorithm"`
	Challenge  string  `json:"challenge"`
	Salt       string  `json:"salt"`
	Signature  string  `json:"signature"`
	Difficulty int     `json:"difficulty"`
	ExpireAt   int64   `json:"expire_at"`
	TargetPath string  `json:"target_path"`
}

type PowResponseHeader struct {
	Algorithm  string `json:"algorithm"`
	Challenge  string `json:"challenge"`
	Salt       string `json:"salt"`
	Answer     int    `json:"answer"`
	Signature  string `json:"signature"`
	TargetPath string `json:"target_path"`
}

type runnerResult struct {
	Answer int `json:"answer"`
}

func SolvePow(challenge Challenge, scriptDir string) (string, error) {
	chalBytes, err := json.Marshal(challenge)
	if err != nil {
		return "", fmt.Errorf("failed to marshal challenge: %w", err)
	}

	runnerPath := filepath.Join(scriptDir, "wasm", "pow_runner.js")
	cmd := exec.Command("node", runnerPath, string(chalBytes))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("pow runner failed: %v, output: %s", err, string(output))
	}

	var res runnerResult
	if err := json.Unmarshal(output, &res); err != nil {
		return "", fmt.Errorf("failed to parse pow output: %v, raw: %s", err, string(output))
	}

	headerObj := PowResponseHeader{
		Algorithm:  challenge.Algorithm,
		Challenge:  challenge.Challenge,
		Salt:       challenge.Salt,
		Answer:     res.Answer,
		Signature:  challenge.Signature,
		TargetPath: challenge.TargetPath,
	}

	headerBytes, err := json.Marshal(headerObj)
	if err != nil {
		return "", fmt.Errorf("failed to marshal pow header obj: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(headerBytes)
	return encoded, nil
}

func GetBaseDir() string {
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		return filepath.Dir(filepath.Dir(filename))
	}
	return "."
}
