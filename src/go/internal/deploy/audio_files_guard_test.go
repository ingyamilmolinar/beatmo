package deploy

// No audio file is ever committed or shipped. Reference recordings are local
// dev tooling that live in $BEATMO_WAV_REFS (default ~/.beatmo/wav-refs, see
// internal/wavref) and every instrument the game plays is synthesized, so
// there is nothing to license and nothing that may reach git or the bucket.
//
// Four layers, each tested here:
//   - .gitignore hides audio of every extension in every letter case (sample
//     libraries ship .WAV / .Wav / .MP3; the old `*.wav` rule missed them and
//     left a 17,996-file local sample library one `git add -A` from the repo);
//   - the pre-commit hook (scripts/precommit/no-audio-files.sh) rejects a
//     staged audio file, which is how `git add -f` gets caught;
//   - this test fails `make test` on any audio file git tracks or has staged;
//   - the deploy script refuses to upload one.

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// audioExts is every audio container extension the guards cover. It must
// stay in step with .gitignore, scripts/precommit/no-audio-files.sh and the
// find(1) regex in scripts/deploy-wasm-gcs.sh; TestAudioExtensionListsAgree
// keeps the shell copies honest.
var audioExts = []string{
	"wav", "wave", "mp3", "aif", "aiff", "aifc", "flac",
	"ogg", "oga", "opus", "m4a", "aac", "wma", "caf",
}

var audioPathRE = regexp.MustCompile(`(?i)\.(` + strings.Join(audioExts, "|") + `)$`)

func isAudioPath(p string) bool { return audioPathRE.MatchString(p) }

// git runs a git command in dir. It skips the test when git or a work tree
// is unavailable (a source tarball has neither), never fails it.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if strings.Contains(stderr.String(), "not a git repository") {
			t.Skip("not a git work tree")
		}
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return string(out)
}

func splitNUL(s string) []string {
	var out []string
	for _, p := range strings.Split(s, "\x00") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// TestNoAudioFilesInRepository fails on any audio file git tracks or has
// staged. A tracked file already deleted from the work tree is a pending
// removal -- the fix in progress -- and passes; a STAGED add/modify fails even
// then, because that version is what the next commit would record.
func TestNoAudioFilesInRepository(t *testing.T) {
	root := repoRoot(t)
	var offenders []string
	for _, p := range splitNUL(git(t, root, "ls-files", "-z")) {
		if !isAudioPath(p) {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, p)); err == nil {
			offenders = append(offenders, p+" (tracked)")
		}
	}
	for _, p := range splitNUL(git(t, root, "diff", "--cached", "--name-only", "--diff-filter=ACMR", "-z")) {
		if isAudioPath(p) {
			offenders = append(offenders, p+" (staged)")
		}
	}
	if len(offenders) > 0 {
		t.Errorf("audio files must never be committed or shipped -- reference audio lives in "+
			"$BEATMO_WAV_REFS (default ~/.beatmo/wav-refs), never in the repo:\n  %s\n"+
			"Move each file out of the tree and drop it from the index:\n"+
			"  git rm --cached -- <file>",
			strings.Join(offenders, "\n  "))
	}
}

// TestGitignoreHidesEveryAudioFile checks the PATTERNS (--no-index), so it
// holds whether or not any such file exists or is tracked.
func TestGitignoreHidesEveryAudioFile(t *testing.T) {
	root := repoRoot(t)
	dirs := []string{"", "src/go/internal/audio/testdata/refs/", "src/js/", "assets/Kick/Akai 5000/"}
	var probes []string
	for _, ext := range audioExts {
		for _, e := range []string{ext, strings.ToUpper(ext), strings.ToUpper(ext[:1]) + ext[1:]} {
			for _, d := range dirs {
				probes = append(probes, d+"take."+e)
			}
		}
	}
	cmd := exec.Command("git", "check-ignore", "--no-index", "--stdin", "-z")
	cmd.Dir = root
	cmd.Stdin = strings.NewReader(strings.Join(probes, "\x00") + "\x00")
	out, err := cmd.Output()
	var exitErr *exec.ExitError
	if err != nil && (!errors.As(err, &exitErr) || exitErr.ExitCode() != 1) {
		git(t, root, "rev-parse", "--git-dir") // skips outside a work tree
		t.Fatalf("git check-ignore: %v", err)
	}
	ignored := map[string]bool{}
	for _, p := range splitNUL(string(out)) {
		ignored[p] = true
	}
	for _, p := range probes {
		if !ignored[p] {
			t.Errorf(".gitignore does not hide %q -- add a case-insensitive pattern for its extension", p)
		}
	}
}

// TestPreCommitHookRejectsAudio drives the real hook against a scratch repo:
// a force-added audio file (any case) is rejected, an ordinary file and the
// REMOVAL of a tracked audio file are allowed.
func TestPreCommitHookRejectsAudio(t *testing.T) {
	hook := filepath.Join(repoRoot(t), "scripts", "precommit", "no-audio-files.sh")
	if _, err := os.Stat(hook); err != nil {
		t.Fatalf("pre-commit hook missing: %v", err)
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not installed")
	}
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(repo, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runHook := func() error {
		cmd := exec.Command("bash", hook)
		cmd.Dir = repo
		return cmd.Run()
	}
	commit := func() {
		git(t, repo, "-c", "user.email=t@example.invalid", "-c", "user.name=t",
			"commit", "-q", "--no-verify", "-m", "x")
	}

	write("notes.txt")
	git(t, repo, "add", "notes.txt")
	if err := runHook(); err != nil {
		t.Fatalf("hook rejected a commit with no audio: %v", err)
	}
	commit()

	for _, name := range []string{"kick.wav", "Kick.WAV", "loop.Mp3", "pad.aiff", "hit.flac"} {
		write(name)
		git(t, repo, "add", "-f", name)
		if err := runHook(); err == nil {
			t.Errorf("hook accepted a staged %s", name)
		}
		git(t, repo, "rm", "-q", "--cached", name)
	}

	// Removing a previously tracked audio file is the fix, never blocked.
	write("old.wav")
	git(t, repo, "add", "-f", "old.wav")
	commit()
	git(t, repo, "rm", "-q", "old.wav")
	if err := runHook(); err != nil {
		t.Errorf("hook rejected removing a tracked audio file: %v", err)
	}
}

// TestInstallHooksInstallsAudioGuard: `make install-hooks` must install the
// dispatcher, and the dispatcher must run the audio check.
func TestInstallHooksInstallsAudioGuard(t *testing.T) {
	root := repoRoot(t)
	mk := mustRead(t, filepath.Join(root, "Makefile"))
	if !strings.Contains(mk, "cp scripts/precommit/pre-commit .git/hooks/pre-commit") {
		t.Error("make install-hooks does not install scripts/precommit/pre-commit")
	}
	disp := mustRead(t, filepath.Join(root, "scripts", "precommit", "pre-commit"))
	if !strings.Contains(disp, "no-audio-files.sh") {
		t.Error("the pre-commit dispatcher does not run no-audio-files.sh")
	}
}

// TestDeployShipsNoAudioFiles: nothing in the deploy manifest is audio, and
// the script refuses to upload audio that reaches the staging dir anyway.
func TestDeployShipsNoAudioFiles(t *testing.T) {
	script := mustRead(t, filepath.Join(repoRoot(t), "scripts", "deploy-wasm-gcs.sh"))
	for f := range extractDeployManifest(t, script) {
		if isAudioPath(f) {
			t.Errorf("deploy manifest ships audio file %q", f)
		}
	}
	if !strings.Contains(script, "audio files must never be deployed") {
		t.Error("deploy-wasm-gcs.sh lost its staged-audio refusal")
	}
}

// TestAudioExtensionListsAgree keeps the shell copies of the extension list
// (hook regex, deploy find regex) in step with audioExts.
func TestAudioExtensionListsAgree(t *testing.T) {
	root := repoRoot(t)
	want := strings.Join(audioExts, "|")
	for _, rel := range []string{
		filepath.Join("scripts", "precommit", "no-audio-files.sh"),
		filepath.Join("scripts", "deploy-wasm-gcs.sh"),
	} {
		if !strings.Contains(mustRead(t, filepath.Join(root, rel)), want) {
			t.Errorf("%s does not carry the audio extension list %q", rel, want)
		}
	}
}
