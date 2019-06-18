package prompt

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/isacikgoz/gia/editor"
	"github.com/isacikgoz/gitin/term"
	git "github.com/isacikgoz/libgit2-api"
)

// Status holds a list of items used to fill the terminal screen.
type Status struct {
	Repo *git.Repository

	prompt *prompt
}

// Start draws the screen with its list, initializing the cursor to the given position.
func (s *Status) Start(opts *Options) error {
	st, err := s.Repo.LoadStatus()
	if err != nil {
		return err
	}
	items := make([]Item, 0)
	for _, entry := range st.Entities {
		items = append(items, entry)
	}

	l, err := NewList(items, opts.Size)
	if err != nil {
		return err
	}
	controls := make(map[string]string)
	controls["add/reset entry"] = "space"
	controls["show diff"] = "enter"
	controls["add all"] = "a"
	controls["reset all"] = "r"
	controls["hunk stage"] = "p"
	controls["commit"] = "c"
	controls["amend"] = "m"
	controls["discard changes"] = "!"

	opts.SearchLabel = "Files"

	s.prompt = &prompt{
		list:      l,
		opts:      opts,
		keys:      s.onKey,
		selection: s.onSelect,
		info:      s.branchInfo,
		controls:  controls,
	}

	if len(items) == 0 {
		s.printClean()
		return nil
	}

	return s.prompt.start()
}

// return true to terminate
func (s *Status) onSelect() bool {
	s.showDiff()
	return false
}

func (s *Status) onKey(key rune) bool {
	var reqReload bool
	switch key {
	case ' ':
		reqReload = true
		s.addReset()
	case 'p':
		reqReload = true
		s.hunkStage()
	case 'c':
		reqReload = true
		s.doCommit()
	case 'm':
		reqReload = true
		s.doCommitAmend()
	case 'a':
		reqReload = true
		// TODO: check for errors
		addAll(s.Repo)
	case 'r':
		reqReload = true
		resetAll(s.Repo)
	case '!':
		reqReload = true
		s.discardChanges()
	case 'q':
		s.prompt.quit <- true
		return true
	default:
	}
	if reqReload {
		if err := s.reloadStatus(); err != nil {
			return true
		}
	}
	return false
}

// reloads the list
func (s *Status) reloadStatus() error {
	_, idx := s.prompt.list.Items()
	status, err := s.Repo.LoadStatus()
	if err != nil {
		return err
	}
	items := make([]Item, 0)
	for _, entry := range status.Entities {
		items = append(items, entry)
	}
	if len(items) == 0 {
		// this is the case when the working tree is cleaned at runtime
		s.prompt.quit <- true
		s.prompt.exitMsg = workingTreeClean(s.Repo.Head)
		return fmt.Errorf("quit")
	}
	s.prompt.list, err = NewList(items, s.prompt.list.size)
	if err != nil {
		return err
	}
	s.prompt.list.SetCursor(idx)
	return nil
}

// add or reset selected entry
func (s *Status) addReset() error {
	defer s.prompt.render()
	items, idx := s.prompt.list.Items()
	entry := items[idx].(*git.StatusEntry)
	args := []string{"add", "--", entry.String()}
	if entry.Indexed() {
		args = []string{"reset", "HEAD", "--", entry.String()}
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = s.Repo.Path()
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

// open hunk stagin ui
func (s *Status) hunkStage() error {
	defer s.prompt.writer.HideCursor()
	items, idx := s.prompt.list.Items()
	entry := items[idx].(*git.StatusEntry)
	file, err := generateDiffFile(s.Repo, entry)
	if err == nil {
		editor, err := editor.NewEditor(file)
		if err != nil {
			return err
		}
		patches, err := editor.Run()
		if err != nil {
			return err
		}
		for _, patch := range patches {
			if err := applyPatchCmd(s.Repo, entry, patch); err != nil {
				return err
			}
		}
	} else {

	}
	return nil
}

// pop git diff
func (s *Status) showDiff() error {
	items, idx := s.prompt.list.Items()
	if idx == NotFound {
		return fmt.Errorf("there is no item to show diff")
	}
	entry := items[idx].(*git.StatusEntry)
	return popGitCommand(s.Repo, fileStatArgs(entry))
}

func (s *Status) doCommit() error {
	defer s.prompt.writer.HideCursor()

	args := []string{"commit", "--edit", "--quiet"}
	err := popGitCommand(s.Repo, args)
	if err != nil {
		return err
	}
	args, err = lastCommitArgs(s.Repo)
	if err != nil {
		return err
	}
	if err := popGitCommand(s.Repo, args); err != nil {
		return err
	}
	return nil
}

func (s *Status) doCommitAmend() error {
	defer s.prompt.writer.HideCursor()

	args := []string{"commit", "--amend", "--quiet"}
	err := popGitCommand(s.Repo, args)
	if err != nil {
		return err
	}
	args, err = lastCommitArgs(s.Repo)
	if err != nil {
		return err
	}
	if err := popGitCommand(s.Repo, args); err != nil {
		return err
	}
	return nil
}

func (s *Status) branchInfo(item Item) [][]term.Cell {
	b := s.Repo.Head
	return branchInfo(b, true)
}

func (s *Status) discardChanges() error {
	defer s.prompt.render()
	items, idx := s.prompt.list.Items()
	entry := items[idx].(*git.StatusEntry)
	args := []string{"checkout", "--", entry.String()}
	cmd := exec.Command("git", args...)
	cmd.Dir = s.Repo.Path()
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func (s *Status) printClean() {
	writer := term.NewBufferedWriter(os.Stdout)
	for _, line := range workingTreeClean(s.Repo.Head) {
		writer.WriteCells(line)
	}
	writer.Flush()
}