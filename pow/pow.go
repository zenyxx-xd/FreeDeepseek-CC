package pow

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type Challenge struct {
	Algorithm  string `json:"algorithm"`
	Challenge  string `json:"challenge"`
	Salt       string `json:"salt"`
	Signature  string `json:"signature"`
	Difficulty int    `json:"difficulty"`
	ExpireAt   int64  `json:"expire_at"`
	TargetPath string `json:"target_path"`
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

func findRunnerScript(baseDir string) (string, error) {
	candidates := []string{
		filepath.Join(baseDir, "pow_runner.js"),
		filepath.Join(baseDir, "wasm", "pow_runner.js"),
		"/opt/freedeepseek-cc/pow_runner.js",
		"/opt/freedeepseek-cc/wasm/pow_runner.js",
		filepath.Join(os.Getenv("HOME"), "FreeDeepseek-CC", "wasm", "pow_runner.js"),
		filepath.Join(os.Getenv("HOME"), "freedeepseek-go", "wasm", "pow_runner.js"),
	}

	if prefix := os.Getenv("PREFIX"); prefix != "" {
		candidates = append(candidates,
			filepath.Join(prefix, "opt", "freedeepseek-cc", "pow_runner.js"),
			filepath.Join(prefix, "opt", "freedeepseek-cc", "wasm", "pow_runner.js"),
		)
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}

	return "", fmt.Errorf("pow_runner.js not found in candidate paths")
}

func SolvePow(challenge Challenge, scriptDir string) (string, error) {
	chalBytes, err := json.Marshal(challenge)
	if err != nil {
		return "", fmt.Errorf("failed to marshal challenge: %w", err)
	}

	runnerPath, err := findRunnerScript(scriptDir)
	if err != nil {
		return "", err
	}

	cmd := exec.Command("node", runnerPath, string(chalBytes))
	cmd.Dir = filepath.Dir(runnerPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("pow runner failed (%s): %v, output: %s", runnerPath, err, string(output))
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
		return "", fmt.Errorf("failed to marshal pow header: %w", err)
	}

	return base64.StdEncoding.EncodeToString(headerBytes), nil
}
