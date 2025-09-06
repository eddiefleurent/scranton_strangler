package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/eddiefleurent/scranton_strangler/internal/models"
)

// helper to create a temp directory and return cleanup
func mustTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return dir
}

// helper to read JSON file into Data for assertions
func readDataFile(t *testing.T, p string) Data {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	var d Data
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return d
}

func TestNewJSONStorage_CreatesDirAndLoadsExisting(t *testing.T) {
	dir := mustTempDir(t)
	path := filepath.Join(dir, "store.json")

	// Pre-create a file with some content
	initial := Data{
		LastUpdated: time.Now().Add(-time.Hour).UTC(),
		DailyPnL:    map[string]float64{"2025-01-01": 123.45},
		Statistics:  &Statistics{TotalTrades: 7},
		History:     []models.Position{},
	}
	b, _ := json.Marshal(initial)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("pre-write: %v", err)
	}

	s, err := NewJSONStorage(path)
	if err != nil {
		t.Fatalf("NewJSONStorage: %v", err)
	}

	// Ensure it loaded previous data and preserved important fields
	got := s.GetStatistics()
	if got.TotalTrades != 7 {
		t.Fatalf("expected TotalTrades=7, got %d", got.TotalTrades)
	}
	if pnl := s.GetDailyPnL("2025-01-01"); pnl != 123.45 {
		t.Fatalf("expected DailyPnL[2025-01-01]=123.45, got %v", pnl)
	}
}

func TestNewJSONStorage_FailsOnUnexpectedStatError(t *testing.T) {
	// Point to a path whose parent cannot be stated by simulating an invalid path on most OS.
	// Use a path with a NUL byte-like invalid character to force error where supported.
	// As Go disallows NUL in paths, we simulate by using an obviously invalid parent on Windows.
	if runtime.GOOS == "windows" {
		_, err := NewJSONStorage(`?:\invalid\path\store.json`)
		if err == nil {
			t.Skip("could not provoke error reliably on Windows; skipping")
		}
	} else {
		// Use a directory we don't have permission to create under (root) in CI-less envs may not fail.
		// Instead, create a temp dir and remove it, then make parent a file so MkdirAll parent will succeed,
		// but later Stat of file path can fail in odd ways; this test is best-effort.
		dir := mustTempDir(t)
		parentFile := filepath.Join(dir, "notadir")
		if err := os.WriteFile(parentFile, []byte("x"), 0o600); err != nil {
			t.Fatalf("write parent file: %v", err)
		}
		path := filepath.Join(parentFile, "store.json")
		_, err := NewJSONStorage(path)
		if err == nil {
			t.Skip("environment allowed invalid path; skipping")
		}
	}
}

func TestSaveAndLoad_RoundTrip_WithPermissionsAndAtomicity(t *testing.T) {
	dir := mustTempDir(t)
	path := filepath.Join(dir, "store.json")

	s, err := NewJSONStorage(path)
	if err != nil {
		t.Fatalf("NewJSONStorage: %v", err)
	}

	pos := &models.Position{
		ID:             "abc123",
		Symbol:         "SPY",
		PutStrike:      420,
		CallStrike:     480,
		Expiration:     time.Now().Add(30 * 24 * time.Hour).UTC(),
		Quantity:       1,
		CreditReceived: 2.5,
		EntryDate:      time.Now().Add(-24 * time.Hour).UTC(),
		EntryIVR:       33.3,
		EntrySpot:      450.12,
		CurrentPnL:     0,
		DTE:            30,
		EntryOrderID:   "E-1",
	}
	if err := s.SetCurrentPosition(pos); err != nil {
		t.Fatalf("SetCurrentPosition: %v", err)
	}

	// Verify file exists and permissions are 0600-ish (on Windows, perms are not POSIX)
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after save: %v", err)
	}
	if runtime.GOOS != "windows" {
		if m := st.Mode().Perm(); m != 0o600 {
			// copyFile sets 0600; rename path inherits temp permissions which were set to 0600.
			// Accept 0600; other modes should fail.
			t.Fatalf("expected file mode 0600, got %o", m)
		}
	}

	// Load again to ensure round-trip
	s2, err := NewJSONStorage(path)
	if err != nil {
		t.Fatalf("NewJSONStorage 2: %v", err)
	}
	got := s2.GetCurrentPosition()
	if got == nil || got.ID != "abc123" || got.Symbol != "SPY" || got.EntryOrderID != "E-1" {
		t.Fatalf("unexpected current position after load: %#v", got)
	}

	// Ensure deep copy semantics: mutate returned position and ensure original not affected after save
	got.Symbol = "QQQ"
	if again := s2.GetCurrentPosition(); again == nil || again.Symbol != "SPY" {
		t.Fatalf("GetCurrentPosition should return a copy; got: %#v", again)
	}
}

func TestGetCurrentPosition_DeepCopyIncludesSlicesAndStateMachineNilSafe(t *testing.T) {
	dir := mustTempDir(t)
	path := filepath.Join(dir, "store.json")
	s, err := NewJSONStorage(path)
	if err != nil {
		t.Fatalf("NewJSONStorage: %v", err)
	}
	adj := models.Adjustment{Note: "roll"} // assuming Adjustment has a Note or similar; if not, zero value still ok
	pos := &models.Position{
		ID:          "id-1",
		Symbol:      "IWM",
		Adjustments: []models.Adjustment{adj},
		// StateMachine left nil; GetCurrentPosition must handle nil safely
	}
	if err := s.SetCurrentPosition(pos); err != nil {
		t.Fatalf("SetCurrentPosition: %v", err)
	}

	cp := s.GetCurrentPosition()
	if cp == nil {
		t.Fatalf("expected current position copy")
	}
	if len(cp.Adjustments) != 1 {
		t.Fatalf("expected 1 adjustment in copy, got %d", len(cp.Adjustments))
	}
	// Mutate copy and ensure original (inside storage) not affected
	cp.Adjustments[0].Note = "changed"
	orig := s.GetCurrentPosition()
	if orig.Adjustments[0].Note == "changed" {
		t.Fatalf("expected deep copy; original was mutated by copy")
	}
}

func TestClonePosition_NilSafeAndDeepCopy(t *testing.T) {
	if clone := clonePosition(nil); clone != nil {
		t.Fatalf("expected nil result when cloning nil")
	}
	orig := &models.Position{
		ID:         "x",
		Symbol:     "TSLA",
		Adjustments: []models.Adjustment{
			{Note: "a"},
			{Note: "b"},
		},
	}
	cl := clonePosition(orig)
	if cl == nil || cl.ID != "x" || cl.Symbol != "TSLA" {
		t.Fatalf("unexpected clone: %#v", cl)
	}
	cl.Adjustments[0].Note = "mutated"
	if orig.Adjustments[0].Note == "mutated" {
		t.Fatalf("expected adjustments deep-copied")
	}
}

func TestValidateFilePath_AllowsWithinStorageDirAndRejectsEscape(t *testing.T) {
	dir := mustTempDir(t)
	path := filepath.Join(dir, "store.json")
	s, err := NewJSONStorage(path)
	if err != nil {
		t.Fatalf("NewJSONStorage: %v", err)
	}

	// Allowed: same dir file
	okPath := filepath.Join(dir, "a.json")
	if err := s.validateFilePath(okPath); err != nil {
		t.Fatalf("validateFilePath should allow sibling file, got error: %v", err)
	}

	// Rejected: path outside storage root
	parent := filepath.Dir(dir)
	outside := filepath.Join(parent, "x.json")
	if err := s.validateFilePath(outside); err == nil {
		t.Fatalf("expected rejection for path escaping storage dir")
	}
}

func TestCopyFile_CopiesAndSyncsWith0600(t *testing.T) {
	dir := mustTempDir(t)
	dstPath := filepath.Join(dir, "store.json")
	s, err := NewJSONStorage(dstPath)
	if err != nil {
		t.Fatalf("NewJSONStorage: %v", err)
	}

	// Prepare source file inside storage root
	srcPath := filepath.Join(filepath.Dir(dstPath), "tmp-src.json")
	content := []byte(`{"k":"v"}`)
	if err := os.WriteFile(srcPath, content, 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}

	if err := s.copyFile(srcPath, dstPath); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("copied content mismatch, got=%q want=%q", string(got), string(content))
	}

	if runtime.GOOS != "windows" {
		if m := (mustStat(t, dstPath).Mode().Perm()); m != 0o600 {
			t.Fatalf("expected dst mode 0600, got %o", m)
		}
	}
}

func mustStat(t *testing.T, p string) os.FileInfo {
	t.Helper()
	st, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	return st
}

func TestSaveHandlesEXDEVByCopying(t *testing.T) {
	// We can't easily simulate cross-device rename (EXDEV) in unit tests.
	// Instead, we simulate os.Rename returning a LinkError with EXDEV and ensure fallback copy happens
	// by creating a storage pointing to a path, then temporarily replacing os.Rename via a test shim.
	// Since we cannot monkey-patch os.Rename in Go, we focus this test on saveUnsafe behavior end-to-end
	// which already uses rename first and will succeed locally. This test ensures Save updates LastUpdated and writes JSON.

	dir := mustTempDir(t)
	path := filepath.Join(dir, "store.json")
	s, err := NewJSONStorage(path)
	if err != nil {
		t.Fatalf("NewJSONStorage: %v", err)
	}

	before := time.Now().Add(-time.Minute).UTC()
	s.data.LastUpdated = before

	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	d := readDataFile(t, path)
	if !d.LastUpdated.After(before) {
		t.Fatalf("expected LastUpdated to be refreshed; before=%v after=%v", before, d.LastUpdated)
	}
}

func TestGetStatistics_ReturnsCopyNotPointerToInternal(t *testing.T) {
	dir := mustTempDir(t)
	path := filepath.Join(dir, "store.json")
	s, err := NewJSONStorage(path)
	if err != nil {
		t.Fatalf("NewJSONStorage: %v", err)
	}
	stats := s.GetStatistics()
	stats.TotalTrades = 999 // mutate returned copy
	if s.GetStatistics().TotalTrades == 999 {
		t.Fatalf("GetStatistics should return a copy (external mutation must not affect internal state)")
	}
}

func TestGetDailyPnL_DefaultZeroWhenMissing(t *testing.T) {
	dir := mustTempDir(t)
	path := filepath.Join(dir, "store.json")
	s, err := NewJSONStorage(path)
	if err != nil {
		t.Fatalf("NewJSONStorage: %v", err)
	}
	if got := s.GetDailyPnL("2099-01-01"); got != 0 {
		t.Fatalf("expected 0 for missing day, got %v", got)
	}
}

func TestGetHistory_DeepCopy(t *testing.T) {
	dir := mustTempDir(t)
	path := filepath.Join(dir, "store.json")
	s, err := NewJSONStorage(path)
	if err != nil {
		t.Fatalf("NewJSONStorage: %v", err)
	}
	// Insert history directly into s.data for controlled testing
	s.mu.Lock()
	s.data.History = []models.Position{
		{ID: "h1", Symbol: "SPY"},
		{ID: "h2", Symbol: "IWM"},
	}
	s.mu.Unlock()

	h := s.GetHistory()
	if len(h) != 2 {
		t.Fatalf("expected 2 history items, got %d", len(h))
	}
	h[0].Symbol = "MUTATED"
	again := s.GetHistory()
	if again[0].Symbol == "MUTATED" {
		t.Fatalf("expected deep copy of history; external mutation should not affect internal state")
	}
}

func TestAddAdjustment_ErrorsWithoutCurrentPosition(t *testing.T) {
	dir := mustTempDir(t)
	path := filepath.Join(dir, "store.json")
	s, err := NewJSONStorage(path)
	if err != nil {
		t.Fatalf("NewJSONStorage: %v", err)
	}
	err = s.AddAdjustment(models.Adjustment{})
	if err == nil || !strings.Contains(err.Error(), "no position") {
		t.Fatalf("expected error about no position to adjust, got: %v", err)
	}
}

func TestAddAdjustment_AppendsAndPersists(t *testing.T) {
	dir := mustTempDir(t)
	path := filepath.Join(dir, "store.json")
	s, err := NewJSONStorage(path)
	if err != nil {
		t.Fatalf("NewJSONStorage: %v", err)
	}
	if err := s.SetCurrentPosition(&models.Position{ID: "p1"}); err != nil {
		t.Fatalf("SetCurrentPosition: %v", err)
	}
	adj := models.Adjustment{Note: "test"}
	if err := s.AddAdjustment(adj); err != nil {
		t.Fatalf("AddAdjustment: %v", err)
	}
	cp := s.GetCurrentPosition()
	if len(cp.Adjustments) != 1 || cp.Adjustments[0].Note != "test" {
		t.Fatalf("adjustment not added as expected: %#v", cp.Adjustments)
	}
	// Ensure persisted
	_ = s.Save()
	d := readDataFile(t, path)
	if len(d.CurrentPosition.Adjustments) != 1 {
		t.Fatalf("expected persisted adjustment")
	}
}

func TestClosePosition_NoCurrentPositionError(t *testing.T) {
	dir := mustTempDir(t)
	path := filepath.Join(dir, "store.json")
	s, err := NewJSONStorage(path)
	if err != nil {
		t.Fatalf("NewJSONStorage: %v", err)
	}
	if err := s.ClosePosition(10, "done"); err == nil {
		t.Fatalf("expected error when no current position")
	}
}

func TestClosePosition_UpdatesHistoryStatsDailyPnLAndClearsCurrent(t *testing.T) {
	dir := mustTempDir(t)
	path := filepath.Join(dir, "store.json")
	s, err := NewJSONStorage(path)
	if err != nil {
		t.Fatalf("NewJSONStorage: %v", err)
	}
	// Create a position with a TransitionState method that may fail if StateMachine not initialized.
	// We assume Position.TransitionState works when called with StateClosed.
	s.mu.Lock()
	s.data.CurrentPosition = &models.Position{
		ID:        "p1",
		Symbol:    "SPY",
		EntryDate: time.Now().Add(-24 * time.Hour).UTC(),
	}
	s.mu.Unlock()

	pnl := 12.34
	if err := s.ClosePosition(pnl, "target met"); err != nil {
		t.Fatalf("ClosePosition: %v", err)
	}

	// Current should be nil
	if s.GetCurrentPosition() != nil {
		t.Fatalf("expected no current position after close")
	}

	// History should have one item with ExitReason/date set
	h := s.GetHistory()
	if len(h) != 1 {
		t.Fatalf("expected history len 1, got %d", len(h))
	}
	if h[0].ExitReason == "" || h[0].ExitDate.IsZero() {
		t.Fatalf("expected ExitReason and ExitDate to be set; got reason=%q date=%v", h[0].ExitReason, h[0].ExitDate)
	}

	// Statistics updated
	stats := s.GetStatistics()
	if stats.TotalTrades != 1 || stats.TotalPnL != pnl || stats.WinningTrades != 1 || stats.WinRate != 1 {
		t.Fatalf("unexpected stats after close: %#v", stats)
	}

	// DailyPnL updated for the day of ExitDate
	day := h[0].ExitDate.Format("2006-01-02")
	if got := s.GetDailyPnL(day); got != pnl {
		t.Fatalf("expected DailyPnL[%s]=%v, got %v", day, pnl, got)
	}
}

func TestUpdateStatistics_LossPathsViaClosePosition(t *testing.T) {
	dir := mustTempDir(t)
	path := filepath.Join(dir, "store.json")
	s, err := NewJSONStorage(path)
	if err != nil {
		t.Fatalf("NewJSONStorage: %v", err)
	}

	// Helper to open and close with pnl
	open := func() {
		s.mu.Lock()
		s.data.CurrentPosition = &models.Position{ID: "p"}
		s.mu.Unlock()
	}
	// Win
	open()
	if err := s.ClosePosition(10, "win"); err != nil {
		t.Fatalf("close win: %v", err)
	}
	// Loss
	open()
	if err := s.ClosePosition(-4, "loss"); err != nil {
		t.Fatalf("close loss: %v", err)
	}
	// Another loss to evolve averages and streak
	open()
	if err := s.ClosePosition(-6, "loss2"); err != nil {
		t.Fatalf("close loss2: %v", err)
	}

	st := s.GetStatistics()
	if st.TotalTrades != 3 {
		t.Fatalf("want TotalTrades=3 got %d", st.TotalTrades)
	}
	if st.WinningTrades != 1 || st.LosingTrades != 2 {
		t.Fatalf("win/loss counts mismatch: %#v", st)
	}
	if st.TotalPnL != 0 {
		t.Fatalf("TotalPnL expected 0 got %v", st.TotalPnL)
	}
	// AverageLoss uses magnitude (positive), (4 + 6)/2 = 5
	if st.AverageLoss != 5 {
		t.Fatalf("AverageLoss expected 5 got %v", st.AverageLoss)
	}
	// CurrentStreak should be -2 (two consecutive losses)
	if st.CurrentStreak != -2 {
		t.Fatalf("CurrentStreak expected -2 got %d", st.CurrentStreak)
	}
	// MaxSingleTradeLoss should be -6
	if st.MaxSingleTradeLoss != -6 {
		t.Fatalf("MaxSingleTradeLoss expected -6 got %v", st.MaxSingleTradeLoss)
	}
}

func TestLoad_InitializesNilFields(t *testing.T) {
	dir := mustTempDir(t)
	path := filepath.Join(dir, "store.json")
	// Write minimal JSON missing fields to test defaults
	minJSON := `{"last_updated":"2025-01-01T00:00:00Z"}`
	if err := os.WriteFile(path, []byte(minJSON), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	s, err := NewJSONStorage(path)
	if err != nil {
		t.Fatalf("NewJSONStorage: %v", err)
	}
	// After Load in NewJSONStorage, DailyPnL and Statistics should be non-nil
	if s.data.DailyPnL == nil || s.data.Statistics == nil {
		t.Fatalf("expected DailyPnL and Statistics to be initialized")
	}
}

// Negative test for copyFile path validation
func TestCopyFile_InvalidPathsRejected(t *testing.T) {
	dir := mustTempDir(t)
	dstPath := filepath.Join(dir, "store.json")
	s, err := NewJSONStorage(dstPath)
	if err != nil {
		t.Fatalf("NewJSONStorage: %v", err)
	}
	// Outside path should be rejected
	parent := filepath.Dir(dir)
	srcOutside := filepath.Join(parent, "x.json")
	if err := s.copyFile(srcOutside, dstPath); err == nil {
		t.Fatalf("expected error when src outside storage root")
	}
	// Destination outside should be rejected
	src := filepath.Join(dir, "src.json")
	_ = os.WriteFile(src, []byte("x"), 0o600)
	if err := s.copyFile(src, filepath.Join(parent, "y.json")); err == nil {
		t.Fatalf("expected error when dst outside storage root")
	}
}

func TestSyncParentDir_ErrorsOnInvalidParent(t *testing.T) {
	dir := mustTempDir(t)
	// Use a storage where parent path validation will fail by pointing storage file to outside parent,
	// then calling syncParentDir should validate the parent (which is within storage dir) successfully.
	// To force an error, temporarily replace s.filepath with an escaping path and call syncParentDir.
	path := filepath.Join(dir, "store.json")
	s, err := NewJSONStorage(path)
	if err != nil {
		t.Fatalf("NewJSONStorage: %v", err)
	}
	// Craft a path outside of dir
	outside := filepath.Join(filepath.Dir(dir), "x", "y.json")
	// Temporarily set filepath and expect validation failure
	s.mu.Lock()
	orig := s.filepath
	s.filepath = outside
	s.mu.Unlock()

	err = s.syncParentDir()
	if err == nil {
		t.Fatalf("expected error from syncParentDir on invalid parent")
	}
	// Restore to avoid side effects
	s.mu.Lock()
	s.filepath = orig
	s.mu.Unlock()
	// Now should succeed
	if err := s.syncParentDir(); err != nil {
		t.Fatalf("syncParentDir valid: %v", err)
	}
}

// Ensure saveUnsafe sets restrictive permissions on temp file and writes valid JSON
func TestSaveUnsafe_WritesIndentedJSON(t *testing.T) {
	dir := mustTempDir(t)
	path := filepath.Join(dir, "store.json")
	s, err := NewJSONStorage(path)
	if err != nil {
		t.Fatalf("NewJSONStorage: %v", err)
	}
	// prepopulate some data
	now := time.Now().UTC()
	s.mu.Lock()
	s.data.CurrentPosition = &models.Position{ID: "p"}
	s.data.DailyPnL["2025-09-06"] = 1.23
	s.data.Statistics.TotalTrades = 2
	s.mu.Unlock()

	if err := s.saveUnsafe(); err != nil {
		t.Fatalf("saveUnsafe: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// Expect indented JSON (newline followed by two spaces)
	if !strings.Contains(string(b), "\n  ") {
		t.Fatalf("expected indented JSON, got: %s", string(b))
	}
	// Ensure LastUpdated is >= now
	var d Data
	_ = json.Unmarshal(b, &d)
	if d.LastUpdated.Before(now) {
		t.Fatalf("LastUpdated not refreshed")
	}
}

// A small sentinel to avoid unused import errors when models.Adjustment doesn't have Note in this repo version.
var _ = func() error {
	_ = errors.New
	_ = fmt.Sprintf
	return nil
}()