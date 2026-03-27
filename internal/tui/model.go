package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/thumbsu/patchdeck/internal/commitmodel"
	"github.com/thumbsu/patchdeck/internal/diffmodel"
	"github.com/thumbsu/patchdeck/internal/navigation"
	"github.com/thumbsu/patchdeck/internal/scanner"
	"github.com/thumbsu/patchdeck/internal/statusmodel"
)

const (
	paneWorktrees = iota
	paneFiles
	paneCommitFiles
	paneDiff
)

const (
	centerFiles = iota
	centerCommits
	centerCommitFiles
)

type rootLoadedMsg struct {
	root string
	refs []scanner.WorktreeRef
	err  error
}

type statusLoadedMsg struct {
	path   string
	status statusmodel.WorktreeStatus
}

type diffLoadedMsg struct {
	worktreePath string
	targetKey    string
	preview      diffmodel.DiffPreview
	err          error
}

type commitsLoadedMsg struct {
	path    string
	commits []commitmodel.Commit
	err     error
}

type commitFilesLoadedMsg struct {
	worktreePath string
	commitHash   string
	files        []commitmodel.CommitFile
	err          error
}

type execFinishedMsg struct {
	message string
	err     error
}

type clearFlashMsg struct{}

type Model struct {
	repoArg string

	root           string
	refs           map[string]scanner.WorktreeRef
	statuses       map[string]statusmodel.WorktreeStatus
	loading        map[string]bool
	commits        map[string][]commitmodel.Commit
	commitErr      map[string]string
	commitFiles    map[string][]commitmodel.CommitFile
	commitFilesErr map[string]string

	selectedWorktreePath   string
	selectedFilePath       string
	selectedCommitHash     string
	selectedCommitFilePath string

	worktreeOffset   int
	fileOffset       int
	commitOffset     int
	commitFileOffset int

	diff        diffmodel.DiffPreview
	diffLoading bool
	diffErr     string
	diffScroll  int
	diffViewKey string

	currentPane int
	centerMode  int
	width       int
	height      int

	refreshing  bool
	flash       string
	flashError  bool
	globalError string
}

func Run(repoArg string) error {
	model := New(repoArg)
	program := tea.NewProgram(model, tea.WithAltScreen())
	_, err := program.Run()
	return err
}

func New(repoArg string) Model {
	return Model{
		repoArg:        repoArg,
		refs:           map[string]scanner.WorktreeRef{},
		statuses:       map[string]statusmodel.WorktreeStatus{},
		loading:        map[string]bool{},
		commits:        map[string][]commitmodel.Commit{},
		commitErr:      map[string]string{},
		commitFiles:    map[string][]commitmodel.CommitFile{},
		commitFilesErr: map[string]string{},
		centerMode:     centerFiles,
	}
}

func (m Model) Init() tea.Cmd {
	return m.refreshCmd()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case rootLoadedMsg:
		m.refreshing = false
		if msg.err != nil {
			m.globalError = msg.err.Error()
			return m, nil
		}
		m.root = msg.root
		m.globalError = ""
		nextRefs := make(map[string]scanner.WorktreeRef, len(msg.refs))
		nextLoading := make(map[string]bool, len(msg.refs))
		for _, ref := range msg.refs {
			nextRefs[ref.Path] = ref
			nextLoading[ref.Path] = true
			if old, ok := m.statuses[ref.Path]; ok {
				old.Ref = ref
				old.Loading = true
				m.statuses[ref.Path] = old
			} else {
				m.statuses[ref.Path] = statusmodel.WorktreeStatus{Ref: ref, Loading: true}
			}
		}
		for path := range m.statuses {
			if _, ok := nextRefs[path]; !ok {
				delete(m.statuses, path)
			}
		}
		m.refs = nextRefs
		m.loading = nextLoading
		m.ensureSelection()

		cmds := make([]tea.Cmd, 0, len(msg.refs))
		for _, ref := range msg.refs {
			cmds = append(cmds, loadStatusCmd(ref))
			cmds = append(cmds, loadCommitsCmd(ref))
		}
		return m, tea.Batch(cmds...)
	case statusLoadedMsg:
		delete(m.loading, msg.path)
		msg.status.Loading = false
		msg.status.Loaded = true
		m.statuses[msg.path] = msg.status
		m.ensureSelection()
		return m, m.loadDiffForSelection()
	case commitsLoadedMsg:
		if msg.err != nil {
			m.commitErr[msg.path] = msg.err.Error()
		} else {
			m.commits[msg.path] = msg.commits
			delete(m.commitErr, msg.path)
		}
		m.ensureSelection()
		if m.centerMode == centerCommits && msg.path == m.selectedWorktreePath {
			return m, m.loadDiffForSelection()
		}
		return m, nil
	case commitFilesLoadedMsg:
		key := commitFilesKey(msg.worktreePath, msg.commitHash)
		if msg.err != nil {
			m.commitFilesErr[key] = msg.err.Error()
		} else {
			m.commitFiles[key] = msg.files
			delete(m.commitFilesErr, key)
		}
		m.ensureSelection()
		if m.centerMode == centerCommitFiles && msg.worktreePath == m.selectedWorktreePath && msg.commitHash == m.selectedCommitHash {
			return m, m.loadDiffForSelection()
		}
		return m, nil
	case diffLoadedMsg:
		if msg.err != nil {
			m.diffErr = msg.err.Error()
			m.diffLoading = false
			m.flash = "diff unavailable"
			m.flashError = true
			return m, flashCmd()
		}
		if msg.worktreePath != m.selectedWorktreePath || msg.targetKey != m.currentDiffTargetKey() {
			return m, nil
		}
		previousKey := m.diffViewKey
		m.diff = msg.preview
		m.diffErr = ""
		m.diffLoading = false
		m.diffViewKey = msg.targetKey
		if previousKey != msg.targetKey {
			m.diffScroll = 0
		}
		return m, nil
	case execFinishedMsg:
		if msg.err != nil {
			m.flash = msg.err.Error()
			m.flashError = true
			return m, flashCmd()
		}
		m.flash = msg.message
		m.flashError = false
		return m, flashCmd()
	case clearFlashMsg:
		m.flash = ""
		m.flashError = false
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) View() string {
	if m.globalError != "" {
		return renderError(m.globalError)
	}

	if m.width <= 0 || m.height <= 0 {
		return "loading..."
	}

	worktreeRows := m.sortedRows()
	currentStatus, _ := m.currentStatus()
	files := currentStatus.ChangedFiles
	commits := m.currentCommits()

	header := m.renderHeader(worktreeRows)
	footer := m.renderFooter()

	paneContentHeight := m.paneContentHeight()

	switch {
	case m.width >= 120:
		return lipgloss.JoinVertical(lipgloss.Left, header, m.renderWide(worktreeRows, files, commits, paneContentHeight), footer)
	case m.width >= 80:
		return lipgloss.JoinVertical(lipgloss.Left, header, m.renderMedium(worktreeRows, files, commits, paneContentHeight), footer)
	default:
		return lipgloss.JoinVertical(lipgloss.Left, header, m.renderNarrow(worktreeRows, files, commits, paneContentHeight), footer)
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "tab":
		m.currentPane = m.nextPane()
		return m, nil
	case "shift+tab":
		m.currentPane = m.prevPane()
		return m, nil
	case "r":
		m.refreshing = true
		return m, m.refreshCmd()
	case "f":
		m.centerMode = centerFiles
		m.currentPane = min(m.currentPane, paneFiles)
		m.ensureSelection()
		return m, m.loadDiffForSelection()
	case "c":
		m.centerMode = centerCommits
		m.currentPane = min(m.currentPane, paneFiles)
		m.ensureSelection()
		return m, tea.Batch(m.loadCommitFilesForSelection(), m.loadDiffForSelection())
	case "j":
		return m.moveDown()
	case "k":
		return m.moveUp()
	case "ctrl+d", "pgdown":
		return m.pageDown()
	case "ctrl+u", "pgup":
		return m.pageUp()
	case "g", "home":
		return m.toTop()
	case "G", "end":
		return m.toBottom()
	case "h", "backspace", "-":
		if m.currentPane == paneFiles && m.centerMode == centerCommitFiles {
			m.centerMode = centerCommits
			m.ensureSelection()
			return m, m.loadDiffForSelection()
		}
		if m.currentPane == paneCommitFiles && m.centerMode == centerCommitFiles {
			m.currentPane = paneFiles
			return m, m.loadDiffForSelection()
		}
		if m.currentPane > paneWorktrees {
			m.currentPane = m.prevPane()
		}
		return m, nil
	case "l", "enter":
		if m.currentPane == paneFiles && m.centerMode == centerCommits {
			m.centerMode = centerCommitFiles
			m.currentPane = paneCommitFiles
			m.ensureSelection()
			cmd := m.loadCommitFilesForSelection()
			if cmd == nil {
				return m, m.forceCommitFileDiffSelection()
			}
			return m, tea.Batch(cmd, m.forceCommitFileDiffSelection())
		}
		if m.currentPane < paneDiff {
			m.currentPane = m.nextPane()
		}
		if m.currentPane == paneDiff {
			return m, m.loadDiffForSelection()
		}
		return m, nil
	case "o":
		targetPath, ok := m.currentOpenPath()
		if !ok {
			return m, nil
		}
		status, ok := m.currentStatus()
		if !ok {
			return m, nil
		}
		cmd, target := navigation.EditorCommand(status.Ref.Path, targetPath)
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
			return execFinishedMsg{
				message: "opened in editor: " + target.AbsPath,
				err:     err,
			}
		})
	case "w":
		status, ok := m.currentStatus()
		if !ok {
			return m, nil
		}
		cmd, target := navigation.ShellCommand(status.Ref.Path)
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
			return execFinishedMsg{
				message: "returned from shell: " + target.AbsPath,
				err:     err,
			}
		})
	case "]":
		return m.jumpConflict(1)
	case "[":
		return m.jumpConflict(-1)
	case "n":
		return m.jumpPriorityCenterItem(1)
	case "N":
		return m.jumpPriorityCenterItem(-1)
	}

	return m, nil
}

func (m Model) moveDown() (Model, tea.Cmd) {
	rows := m.sortedRows()
	switch m.currentPane {
	case paneWorktrees:
		if len(rows) == 0 {
			return m, nil
		}
		idx := m.selectedWorktreeIndex(rows)
		if idx < len(rows)-1 {
			return m.selectWorktree(rows[idx+1].Ref.Path)
		}
	case paneFiles:
		if m.centerMode == centerFiles {
			status, ok := m.currentStatus()
			if !ok || len(status.ChangedFiles) == 0 {
				return m, nil
			}
			idx := m.selectedFileIndex(status.ChangedFiles)
			if idx < len(status.ChangedFiles)-1 {
				m.selectedFilePath = status.ChangedFiles[idx+1].Path
				m.fileOffset = followOffset(m.fileOffset, idx+1, m.listVisibleItemCount())
				return m, m.loadDiffForSelection()
			}
		} else {
			commits := m.currentCommits()
			if len(commits) == 0 {
				return m, nil
			}
			idx := m.selectedCommitIndex(commits)
			if idx < len(commits)-1 {
				return m.selectCommit(commits[idx+1].Hash)
			}
		}
	case paneCommitFiles:
		files := m.currentCommitFiles()
		if len(files) == 0 {
			return m, nil
		}
		idx := m.selectedCommitFileIndex(files)
		if idx < len(files)-1 {
			m.selectedCommitFilePath = files[idx+1].Path
			m.commitFileOffset = followOffset(m.commitFileOffset, idx+1, m.listVisibleItemCount())
			return m, m.loadDiffForSelection()
		}
	case paneDiff:
		m.diffScroll++
	}
	return m, nil
}

func (m Model) moveUp() (Model, tea.Cmd) {
	rows := m.sortedRows()
	switch m.currentPane {
	case paneWorktrees:
		if len(rows) == 0 {
			return m, nil
		}
		idx := m.selectedWorktreeIndex(rows)
		if idx > 0 {
			return m.selectWorktree(rows[idx-1].Ref.Path)
		}
	case paneFiles:
		if m.centerMode == centerFiles {
			status, ok := m.currentStatus()
			if !ok || len(status.ChangedFiles) == 0 {
				return m, nil
			}
			idx := m.selectedFileIndex(status.ChangedFiles)
			if idx > 0 {
				m.selectedFilePath = status.ChangedFiles[idx-1].Path
				m.fileOffset = followOffset(m.fileOffset, idx-1, m.listVisibleItemCount())
				return m, m.loadDiffForSelection()
			}
		} else {
			commits := m.currentCommits()
			if len(commits) == 0 {
				return m, nil
			}
			idx := m.selectedCommitIndex(commits)
			if idx > 0 {
				return m.selectCommit(commits[idx-1].Hash)
			}
		}
	case paneCommitFiles:
		files := m.currentCommitFiles()
		if len(files) == 0 {
			return m, nil
		}
		idx := m.selectedCommitFileIndex(files)
		if idx > 0 {
			m.selectedCommitFilePath = files[idx-1].Path
			m.commitFileOffset = followOffset(m.commitFileOffset, idx-1, m.listVisibleItemCount())
			return m, m.loadDiffForSelection()
		}
	case paneDiff:
		if m.diffScroll > 0 {
			m.diffScroll--
		}
	}
	return m, nil
}

func (m Model) pageDown() (Model, tea.Cmd) {
	if m.currentPane != paneDiff {
		return m.moveDown()
	}
	step := m.diffPageStep()
	m.diffScroll += step
	return m, nil
}

func (m Model) pageUp() (Model, tea.Cmd) {
	if m.currentPane != paneDiff {
		return m.moveUp()
	}
	step := m.diffPageStep()
	m.diffScroll -= step
	if m.diffScroll < 0 {
		m.diffScroll = 0
	}
	return m, nil
}

func (m Model) toTop() (Model, tea.Cmd) {
	switch m.currentPane {
	case paneWorktrees:
		rows := m.sortedRows()
		if len(rows) > 0 {
			return m.selectWorktree(rows[0].Ref.Path)
		}
	case paneFiles:
		if m.centerMode == centerFiles {
			status, ok := m.currentStatus()
			if ok && len(status.ChangedFiles) > 0 {
				m.selectedFilePath = status.ChangedFiles[0].Path
				m.fileOffset = 0
				return m, m.loadDiffForSelection()
			}
		} else {
			commits := m.currentCommits()
			if len(commits) > 0 {
				return m.selectCommit(commits[0].Hash)
			}
		}
	case paneCommitFiles:
		files := m.currentCommitFiles()
		if len(files) > 0 {
			m.selectedCommitFilePath = files[0].Path
			m.commitFileOffset = 0
			return m, m.loadDiffForSelection()
		}
	case paneDiff:
		m.diffScroll = 0
	}
	return m, nil
}

func (m Model) toBottom() (Model, tea.Cmd) {
	switch m.currentPane {
	case paneWorktrees:
		rows := m.sortedRows()
		if len(rows) > 0 {
			return m.selectWorktree(rows[len(rows)-1].Ref.Path)
		}
	case paneFiles:
		if m.centerMode == centerFiles {
			status, ok := m.currentStatus()
			if ok && len(status.ChangedFiles) > 0 {
				m.selectedFilePath = status.ChangedFiles[len(status.ChangedFiles)-1].Path
				m.fileOffset = bottomOffset(len(status.ChangedFiles), m.listVisibleItemCount())
				return m, m.loadDiffForSelection()
			}
		} else {
			commits := m.currentCommits()
			if len(commits) > 0 {
				return m.selectCommit(commits[len(commits)-1].Hash)
			}
		}
	case paneCommitFiles:
		files := m.currentCommitFiles()
		if len(files) > 0 {
			m.selectedCommitFilePath = files[len(files)-1].Path
			m.commitFileOffset = bottomOffset(len(files), m.listVisibleItemCount())
			return m, m.loadDiffForSelection()
		}
	case paneDiff:
		m.diffScroll = 1 << 30
	}
	return m, nil
}

func (m Model) jumpConflict(direction int) (Model, tea.Cmd) {
	rows := m.sortedRows()
	if len(rows) == 0 {
		return m, nil
	}
	start := m.selectedWorktreeIndex(rows)
	for offset := 1; offset <= len(rows); offset++ {
		idx := (start + direction*offset + len(rows)) % len(rows)
		if rows[idx].Status.ConflictedCount > 0 || rows[idx].Ref.Prunable || rows[idx].Status.ScanError != "" {
			return m.selectWorktree(rows[idx].Ref.Path)
		}
	}
	return m, nil
}

func (m Model) jumpPriorityCenterItem(direction int) (Model, tea.Cmd) {
	if m.centerMode == centerCommits {
		commits := m.currentCommits()
		if len(commits) == 0 {
			return m, nil
		}
		start := m.selectedCommitIndex(commits)
		for offset := 1; offset <= len(commits); offset++ {
			idx := (start + direction*offset + len(commits)) % len(commits)
			if idx >= 0 && idx < len(commits) {
				if m.currentPane < paneFiles {
					m.currentPane = paneFiles
				}
				m.commitOffset = followOffset(m.commitOffset, idx, m.listVisibleItemCount())
				return m.selectCommit(commits[idx].Hash)
			}
		}
		return m, nil
	}
	if m.centerMode == centerCommitFiles {
		files := m.currentCommitFiles()
		if len(files) == 0 {
			return m, nil
		}
		start := m.selectedCommitFileIndex(files)
		for offset := 1; offset <= len(files); offset++ {
			idx := (start + direction*offset + len(files)) % len(files)
			if idx >= 0 && idx < len(files) {
				m.selectedCommitFilePath = files[idx].Path
				if m.currentPane < paneFiles {
					m.currentPane = paneFiles
				}
				m.commitFileOffset = followOffset(m.commitFileOffset, idx, m.listVisibleItemCount())
				return m, m.loadDiffForSelection()
			}
		}
		return m, nil
	}

	status, ok := m.currentStatus()
	if !ok || len(status.ChangedFiles) == 0 {
		return m, nil
	}

	start := m.selectedFileIndex(status.ChangedFiles)
	for offset := 1; offset <= len(status.ChangedFiles); offset++ {
		idx := (start + direction*offset + len(status.ChangedFiles)) % len(status.ChangedFiles)
		file := status.ChangedFiles[idx]
		if file.Conflicted || file.Untracked || file.Deleted || file.IsDir {
			m.selectedFilePath = file.Path
			if m.currentPane < paneFiles {
				m.currentPane = paneFiles
			}
			m.fileOffset = followOffset(m.fileOffset, idx, m.listVisibleItemCount())
			return m, m.loadDiffForSelection()
		}
	}
	return m, nil
}

func (m *Model) ensureSelection() {
	rows := m.sortedRows()
	if len(rows) == 0 {
		m.selectedWorktreePath = ""
		m.selectedFilePath = ""
		return
	}

	found := false
	for _, row := range rows {
		if row.Ref.Path == m.selectedWorktreePath {
			found = true
			break
		}
	}
	if !found {
		m.selectedWorktreePath = rows[0].Ref.Path
	}
	m.worktreeOffset = followOffset(m.worktreeOffset, m.selectedWorktreeIndex(rows), m.listVisibleItemCount())

	commits := m.currentCommits()
	if len(commits) == 0 {
		m.selectedCommitHash = ""
	} else {
		foundCommit := false
		for _, commit := range commits {
			if commit.Hash == m.selectedCommitHash {
				foundCommit = true
				break
			}
		}
		if !foundCommit {
			m.selectedCommitHash = commits[0].Hash
		}
	}
	m.commitOffset = followOffset(m.commitOffset, m.selectedCommitIndex(commits), m.listVisibleItemCount())

	commitFiles := m.currentCommitFiles()
	if len(commitFiles) == 0 {
		m.selectedCommitFilePath = ""
	} else {
		foundCommitFile := false
		for _, file := range commitFiles {
			if file.Path == m.selectedCommitFilePath {
				foundCommitFile = true
				break
			}
		}
		if !foundCommitFile {
			m.selectedCommitFilePath = commitFiles[0].Path
		}
	}
	m.commitFileOffset = followOffset(m.commitFileOffset, m.selectedCommitFileIndex(commitFiles), m.listVisibleItemCount())

	status, ok := m.currentStatus()
	if !ok || len(status.ChangedFiles) == 0 {
		m.selectedFilePath = ""
		return
	}

	for _, file := range status.ChangedFiles {
		if file.Path == m.selectedFilePath {
			m.fileOffset = followOffset(m.fileOffset, m.selectedFileIndex(status.ChangedFiles), m.listVisibleItemCount())
			return
		}
	}
	m.selectedFilePath = status.ChangedFiles[0].Path
	m.fileOffset = 0
}

func (m Model) currentStatus() (statusmodel.WorktreeStatus, bool) {
	status, ok := m.statuses[m.selectedWorktreePath]
	return status, ok
}

func (m Model) currentFile() (statusmodel.WorktreeStatus, *statusmodel.ChangedFile) {
	status, ok := m.currentStatus()
	if !ok {
		return statusmodel.WorktreeStatus{}, nil
	}
	for i := range status.ChangedFiles {
		if status.ChangedFiles[i].Path == m.selectedFilePath {
			return status, &status.ChangedFiles[i]
		}
	}
	if len(status.ChangedFiles) == 0 {
		return status, nil
	}
	return status, &status.ChangedFiles[0]
}

func (m Model) currentOpenPath() (string, bool) {
	switch m.centerMode {
	case centerCommitFiles:
		file := m.currentCommitFile()
		if file == nil {
			return "", false
		}
		return file.Path, true
	case centerFiles:
		_, file := m.currentFile()
		if file == nil {
			return "", false
		}
		return file.Path, true
	default:
		return "", false
	}
}

func (m Model) currentCommits() []commitmodel.Commit {
	return m.commits[m.selectedWorktreePath]
}

func (m Model) currentCommit() *commitmodel.Commit {
	commits := m.currentCommits()
	for i := range commits {
		if commits[i].Hash == m.selectedCommitHash {
			return &commits[i]
		}
	}
	if len(commits) == 0 {
		return nil
	}
	return &commits[0]
}

func (m Model) currentCommitFiles() []commitmodel.CommitFile {
	return m.commitFiles[commitFilesKey(m.selectedWorktreePath, m.selectedCommitHash)]
}

func (m Model) currentCommitFile() *commitmodel.CommitFile {
	files := m.currentCommitFiles()
	if len(files) > 0 && m.selectedCommitFilePath == "" {
		return &files[0]
	}
	for i := range files {
		if files[i].Path == m.selectedCommitFilePath {
			return &files[i]
		}
	}
	if len(files) == 0 {
		return nil
	}
	return &files[0]
}

func (m Model) loadDiffForSelection() tea.Cmd {
	if m.centerMode == centerCommits {
		commit := m.currentCommit()
		if commit == nil {
			m.diffLoading = false
			m.diff = diffmodel.DiffPreview{}
			m.diffViewKey = ""
			return nil
		}
		m.diffLoading = true
		return loadCommitDiffCmd(m.selectedWorktreePath, *commit)
	}
	if m.centerMode == centerCommitFiles {
		commit := m.currentCommit()
		file := m.currentCommitFile()
		if commit == nil || file == nil {
			m.diffLoading = false
			m.diff = diffmodel.DiffPreview{}
			m.diffViewKey = ""
			return nil
		}
		m.diffLoading = true
		return loadCommitFileDiffCmd(m.selectedWorktreePath, *commit, *file)
	}

	status, file := m.currentFile()
	if file == nil {
		m.diffLoading = false
		m.diff = diffmodel.DiffPreview{}
		m.diffViewKey = ""
		return nil
	}
	m.diffLoading = true
	return loadDiffCmd(status.Ref.Path, *file)
}

func (m Model) currentDiffTargetKey() string {
	if m.centerMode == centerCommits {
		return "commit:" + m.selectedCommitHash
	}
	if m.centerMode == centerCommitFiles {
		return "commit-file:" + m.selectedCommitHash + ":" + m.selectedCommitFilePath
	}
	return "file:" + m.selectedFilePath
}

type row struct {
	Ref    scanner.WorktreeRef
	Status statusmodel.WorktreeStatus
}

func (m Model) sortedRows() []row {
	rows := make([]row, 0, len(m.refs))
	for path, ref := range m.refs {
		status := m.statuses[path]
		status.Ref = ref
		rows = append(rows, row{Ref: ref, Status: status})
	}

	sort.SliceStable(rows, func(i, j int) bool {
		left := rowPriority(rows[i])
		right := rowPriority(rows[j])
		if left != right {
			return left > right
		}
		leftCount := len(rows[i].Status.ChangedFiles)
		rightCount := len(rows[j].Status.ChangedFiles)
		if leftCount != rightCount {
			return leftCount > rightCount
		}
		return rows[i].Ref.Path < rows[j].Ref.Path
	})

	return rows
}

func rowPriority(item row) int {
	switch {
	case item.Ref.Prunable:
		return 6
	case item.Status.ScanError != "":
		return 5
	case item.Status.ConflictedCount > 0:
		return 4
	case len(item.Status.ChangedFiles) > 0:
		return 3
	case item.Status.Loading:
		return 2
	default:
		return 1
	}
}

func (m Model) selectedWorktreeIndex(rows []row) int {
	for i, row := range rows {
		if row.Ref.Path == m.selectedWorktreePath {
			return i
		}
	}
	return 0
}

func (m Model) selectedFileIndex(files []statusmodel.ChangedFile) int {
	for i, file := range files {
		if file.Path == m.selectedFilePath {
			return i
		}
	}
	return 0
}

func (m Model) selectedCommitIndex(commits []commitmodel.Commit) int {
	for i, commit := range commits {
		if commit.Hash == m.selectedCommitHash {
			return i
		}
	}
	return 0
}

func (m Model) selectedCommitFileIndex(files []commitmodel.CommitFile) int {
	for i, file := range files {
		if file.Path == m.selectedCommitFilePath {
			return i
		}
	}
	return 0
}

func (m Model) refreshCmd() tea.Cmd {
	repoArg := m.repoArg
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		root, refs, err := scanner.Discover(ctx, repoArg)
		return rootLoadedMsg{root: root, refs: refs, err: err}
	}
}

func loadStatusCmd(ref scanner.WorktreeRef) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return statusLoadedMsg{
			path:   ref.Path,
			status: statusmodel.Load(ctx, ref),
		}
	}
}

func loadCommitsCmd(ref scanner.WorktreeRef) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		commits, err := commitmodel.Load(ctx, ref.Path)
		return commitsLoadedMsg{
			path:    ref.Path,
			commits: commits,
			err:     err,
		}
	}
}

func loadCommitFilesCmd(worktreePath string, commitHash string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		files, err := commitmodel.LoadFiles(ctx, worktreePath, commitHash)
		return commitFilesLoadedMsg{
			worktreePath: worktreePath,
			commitHash:   commitHash,
			files:        files,
			err:          err,
		}
	}
}

func loadDiffCmd(worktreePath string, file statusmodel.ChangedFile) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		preview, err := diffmodel.Load(ctx, worktreePath, file)
		return diffLoadedMsg{
			worktreePath: worktreePath,
			targetKey:    "file:" + file.Path,
			preview:      preview,
			err:          err,
		}
	}
}

func loadCommitDiffCmd(worktreePath string, commit commitmodel.Commit) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		preview, err := diffmodel.LoadCommit(ctx, worktreePath, commit)
		return diffLoadedMsg{
			worktreePath: worktreePath,
			targetKey:    "commit:" + commit.Hash,
			preview:      preview,
			err:          err,
		}
	}
}

func loadCommitFileDiffCmd(worktreePath string, commit commitmodel.Commit, file commitmodel.CommitFile) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		preview, err := diffmodel.LoadCommitFile(ctx, worktreePath, commit, file)
		return diffLoadedMsg{
			worktreePath: worktreePath,
			targetKey:    "commit-file:" + commit.Hash + ":" + file.Path,
			preview:      preview,
			err:          err,
		}
	}
}

func commitFilesKey(worktreePath, commitHash string) string {
	return worktreePath + "::" + commitHash
}

func (m Model) loadCommitFilesForSelection() tea.Cmd {
	if m.selectedWorktreePath == "" || m.selectedCommitHash == "" {
		return nil
	}
	key := commitFilesKey(m.selectedWorktreePath, m.selectedCommitHash)
	if _, ok := m.commitFiles[key]; ok {
		return nil
	}
	return loadCommitFilesCmd(m.selectedWorktreePath, m.selectedCommitHash)
}

func (m Model) loadSelectionAfterWorktreeChange() tea.Cmd {
	if m.centerMode == centerCommitFiles || m.centerMode == centerCommits {
		return tea.Batch(m.loadCommitFilesForSelection(), m.loadDiffForSelection())
	}
	return m.loadDiffForSelection()
}

func (m Model) selectWorktree(path string) (Model, tea.Cmd) {
	m.selectedWorktreePath = path
	m.selectedFilePath = ""
	m.selectedCommitHash = ""
	m.selectedCommitFilePath = ""
	m.fileOffset = 0
	m.commitOffset = 0
	m.commitFileOffset = 0
	m.ensureSelection()
	return m, m.loadSelectionAfterWorktreeChange()
}

func (m Model) forceCommitFileDiffSelection() tea.Cmd {
	files := m.currentCommitFiles()
	if len(files) == 0 {
		m.diffLoading = false
		m.diff = diffmodel.DiffPreview{}
		m.diffViewKey = ""
		m.selectedCommitFilePath = ""
		return nil
	}
	m.selectedCommitFilePath = files[0].Path
	return m.loadDiffForSelection()
}

func (m Model) selectCommit(hash string) (Model, tea.Cmd) {
	m.selectedCommitHash = hash
	m.selectedCommitFilePath = ""
	m.diff = diffmodel.DiffPreview{}
	m.diffErr = ""
	m.diffLoading = false
	m.diffViewKey = ""

	cmd := m.loadCommitFilesForSelection()
	if cmd == nil {
		m.ensureSelection()
		return m, m.loadDiffForSelection()
	}
	return m, cmd
}

func (m Model) nextPane() int {
	switch {
	case m.centerMode == centerCommitFiles:
		switch m.currentPane {
		case paneWorktrees:
			return paneFiles
		case paneFiles:
			return paneCommitFiles
		case paneCommitFiles:
			return paneDiff
		default:
			return paneDiff
		}
	default:
		switch m.currentPane {
		case paneWorktrees:
			return paneFiles
		case paneFiles:
			return paneDiff
		default:
			return paneDiff
		}
	}
}

func (m Model) prevPane() int {
	switch {
	case m.centerMode == centerCommitFiles:
		switch m.currentPane {
		case paneDiff:
			return paneCommitFiles
		case paneCommitFiles:
			return paneFiles
		case paneFiles:
			return paneWorktrees
		default:
			return paneWorktrees
		}
	default:
		switch m.currentPane {
		case paneDiff:
			return paneFiles
		case paneFiles:
			return paneWorktrees
		default:
			return paneWorktrees
		}
	}
}

func flashCmd() tea.Cmd {
	return tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
		return clearFlashMsg{}
	})
}

var (
	colorMuted   = lipgloss.Color("241")
	colorBorder  = lipgloss.Color("240")
	colorAccent  = lipgloss.Color("69")
	colorDanger  = lipgloss.Color("203")
	colorSuccess = lipgloss.Color("42")
	colorAdded   = lipgloss.Color("42")
	colorRemoved = lipgloss.Color("203")
	colorHunk    = lipgloss.Color("81")
	colorPath    = lipgloss.Color("250")

	basePaneStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	activePaneStyle = basePaneStyle.Copy().BorderForeground(colorAccent)
	titleStyle      = lipgloss.NewStyle().Bold(true)
	headerTextStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252"))
	mutedStyle      = lipgloss.NewStyle().Foreground(colorMuted)
	selectedStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	dangerStyle     = lipgloss.NewStyle().Foreground(colorDanger).Bold(true)
	successStyle    = lipgloss.NewStyle().Foreground(colorSuccess)
	addedStyle      = lipgloss.NewStyle().Foreground(colorAdded)
	removedStyle    = lipgloss.NewStyle().Foreground(colorRemoved)
	hunkStyle       = lipgloss.NewStyle().Foreground(colorHunk).Bold(true)
	pathStyle       = lipgloss.NewStyle().Foreground(colorPath).Bold(true)
)

func (m Model) renderHeader(rows []row) string {
	conflicts := 0
	dirty := 0
	errors := 0
	for _, row := range rows {
		if row.Status.ConflictedCount > 0 {
			conflicts++
		}
		if row.Status.ScanError != "" || row.Ref.Prunable {
			errors++
		}
		if len(row.Status.ChangedFiles) > 0 {
			dirty++
		}
	}

	parts := []string{"patchdeck"}
	switch {
	case conflicts > 0:
		parts = append(parts, fmt.Sprintf("%d conflicts need attention", conflicts))
	case errors > 0:
		parts = append(parts, fmt.Sprintf("%d broken scans need attention", errors))
	default:
		parts = append(parts, fmt.Sprintf("%d dirty worktrees", dirty))
	}

	if m.refreshing {
		parts = append(parts, "refreshing...")
	}
	if m.flash != "" {
		parts = append(parts, m.flash)
	}

	line := strings.Join(parts, " | ")
	lines := clampLines(strings.Split(line, "\n"), max(8, m.width))
	if m.flashError {
		for i := range lines {
			lines[i] = dangerStyle.Render(lines[i])
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderFooter() string {
	help := "j/k move  h/l panes  f files  c commits  r refresh  o editor  w shell  [/] worktrees  n/N jump  q quit"
	if m.width < 90 {
		help = "j/k move  enter drill  h/back back  f files  c commits  r refresh  q quit"
	}
	switch m.currentPane {
	case paneDiff:
		help += "\npgup/pgdn or ctrl+u/d scroll  g/G top/bottom"
	case paneFiles:
		if m.centerMode == centerCommits {
			help += "\ncommit list = branch commits ahead of base"
		} else if m.centerMode == centerCommitFiles {
			help += "\ncommit files = selected commit, h to go back"
		} else {
			help += "\nselected file stays focused after refresh"
		}
	case paneWorktrees:
		help += "\ntriage queue sorts conflict > scan failure > changed count"
	}
	if m.root != "" {
		help = help + "\nrepo: " + m.root
	}
	lines := clampLines(strings.Split(help, "\n"), max(8, m.width))
	for i := range lines {
		lines[i] = mutedStyle.Render(lines[i])
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderWide(rows []row, files []statusmodel.ChangedFile, commits []commitmodel.Commit, bodyHeight int) string {
	if m.centerMode == centerCommitFiles {
		// 4 panes, each with left+right borders => 8 columns of border chrome.
		total := max(72, m.width-8)
		leftWidth := max(22, total/5)
		commitWidth := max(24, total/4)
		fileWidth := max(24, total/4)
		rightWidth := max(28, total-leftWidth-commitWidth-fileWidth)
		left := m.renderWorktrees(rows, leftWidth, bodyHeight, m.currentPane == paneWorktrees)
		centerLeft := m.renderCommits(commits, commitWidth, bodyHeight, m.currentPane == paneFiles)
		centerRight := m.renderCommitFiles(m.currentCommitFiles(), fileWidth, bodyHeight, m.currentPane == paneCommitFiles)
		right := m.renderDiff(rightWidth, bodyHeight, m.currentPane == paneDiff)
		return lipgloss.JoinHorizontal(lipgloss.Top, left, centerLeft, centerRight, right)
	}

	// 3 panes, each with left+right borders => 6 columns of border chrome.
	total := max(64, m.width-6)
	leftWidth := max(24, total/4)
	centerWidth := max(28, total/3)
	rightWidth := max(32, total-leftWidth-centerWidth)
	left := m.renderWorktrees(rows, leftWidth, bodyHeight, m.currentPane == paneWorktrees)
	center := m.renderCenter(files, commits, centerWidth, bodyHeight, m.currentPane == paneFiles)
	right := m.renderDiff(rightWidth, bodyHeight, m.currentPane == paneDiff)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, center, right)
}

func (m Model) renderMedium(rows []row, files []statusmodel.ChangedFile, commits []commitmodel.Commit, bodyHeight int) string {
	if m.currentPane == paneDiff {
		return m.renderDiff(m.width, bodyHeight, true, "DIFF")
	}
	if m.centerMode == centerCommitFiles {
		total := max(50, m.width-4) // 2 panes => 4 columns of border chrome.
		leftWidth := total / 2
		rightWidth := total - leftWidth
		left := m.renderCommits(commits, leftWidth, bodyHeight, m.currentPane == paneFiles)
		right := m.renderCommitFiles(m.currentCommitFiles(), rightWidth, bodyHeight, m.currentPane == paneCommitFiles)
		return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}
	total := max(50, m.width-4) // 2 panes => 4 columns of border chrome.
	leftWidth := total / 2
	rightWidth := total - leftWidth
	left := m.renderWorktrees(rows, leftWidth, bodyHeight, m.currentPane == paneWorktrees)
	right := m.renderCenter(files, commits, rightWidth, bodyHeight, m.currentPane == paneFiles)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m Model) renderNarrow(rows []row, files []statusmodel.ChangedFile, commits []commitmodel.Commit, bodyHeight int) string {
	switch m.currentPane {
	case paneWorktrees:
		return m.renderWorktrees(rows, m.width, bodyHeight, true, "QUEUE 1/3")
	case paneFiles:
		label := "FILES 2/3"
		if m.centerMode == centerCommits {
			label = "COMMITS 2/3"
		} else if m.centerMode == centerCommitFiles {
			label = "COMMIT FILES 2/3"
		}
		return m.renderCenter(files, commits, m.width, bodyHeight, true, label)
	default:
		return m.renderDiff(m.width, bodyHeight, true, "DIFF 3/3")
	}
}

func (m Model) renderWorktrees(rows []row, width, height int, active bool, stageLabel ...string) string {
	style := basePaneStyle.Width(width)
	if active {
		style = activePaneStyle.Width(width)
	}
	innerWidth := paneInnerWidth(width)
	contentHeight := max(1, height-1)

	title := "WORKTREES"
	if len(stageLabel) > 0 {
		title = stageLabel[0] + "  " + title
	}
	lines := []string{headerTextStyle.Render(title)}
	if len(rows) == 0 {
		body := strings.Join([]string{
			"No worktrees found.",
			mutedStyle.Render("Run from a repo root or pass --repo <path>."),
		}, "\n")
		return style.Render(renderViewport(innerWidth, contentHeight, 0, lines[0], body))
	}

	maxItems := visibleItemCount(height)
	start, end := windowFromOffset(len(rows), m.worktreeOffset, maxItems)
	for _, row := range rows[start:end] {
		prefix := "  "
		lineStyle := lipgloss.NewStyle()
		if row.Ref.Path == m.selectedWorktreePath {
			prefix = "> "
			lineStyle = selectedStyle
		}

		label := row.Ref.Branch
		if label == "" {
			label = filepath.Base(row.Ref.Path)
		}
		statusText, statusStyle := statusBadgeText(row)
		count := fmt.Sprintf("%d files", len(row.Status.ChangedFiles))
		if row.Status.Loading {
			count = "scanning..."
		}
		mainRaw := fmt.Sprintf("%s%-20s %-10s %s", prefix, truncate(label, 20), statusText, count)
		mainRaw = truncate(mainRaw, innerWidth)
		styled := lineStyle.Render(mainRaw)
		if strings.Contains(mainRaw, statusText) {
			styled = strings.Replace(styled, statusText, statusStyle.Render(statusText), 1)
		}
		lines = append(lines, styled)

		reason := row.Status.ReasonLine
		if row.Ref.Prunable && strings.TrimSpace(reason) == "" {
			reason = "broken worktree needs attention"
		}
		if reason != "" {
			lines = append(lines, mutedStyle.Render(truncate("   "+reason, innerWidth)))
		}
	}

	body := strings.Join(lines[1:], "\n")
	return style.Render(renderViewport(innerWidth, contentHeight, 0, lines[0], body))
}

func statusBadgeText(row row) (string, lipgloss.Style) {
	switch {
	case row.Ref.Prunable || row.Status.ScanError != "":
		return "ERROR", dangerStyle
	case row.Status.ConflictedCount > 0:
		return "CONFLICT", dangerStyle
	case len(row.Status.ChangedFiles) > 0:
		return "DIRTY", selectedStyle
	case row.Status.Loading:
		return "SCAN", mutedStyle
	default:
		return "OK", successStyle
	}
}

func (m Model) renderCenter(files []statusmodel.ChangedFile, commits []commitmodel.Commit, width, height int, active bool, stageLabel ...string) string {
	if m.centerMode == centerCommits {
		return m.renderCommits(commits, width, height, active, stageLabel...)
	}
	if m.centerMode == centerCommitFiles {
		return m.renderCommitFiles(m.currentCommitFiles(), width, height, active, stageLabel...)
	}
	return m.renderFiles(files, width, height, active, stageLabel...)
}

func (m Model) renderFiles(files []statusmodel.ChangedFile, width, height int, active bool, stageLabel ...string) string {
	style := basePaneStyle.Width(width)
	if active {
		style = activePaneStyle.Width(width)
	}
	innerWidth := paneInnerWidth(width)
	contentHeight := max(1, height-1)

	title := "FILES"
	if len(stageLabel) > 0 {
		title = stageLabel[0] + "  " + title
	}
	lines := []string{headerTextStyle.Render(title)}
	if len(files) == 0 {
		return style.Render(renderViewport(innerWidth, contentHeight, 0, lines[0], "No changed files in this worktree."))
	}

	maxItems := visibleItemCount(height)
	start, end := windowFromOffset(len(files), m.fileOffset, maxItems)
	for _, file := range files[start:end] {
		prefix := "  "
		lineStyle := lipgloss.NewStyle()
		if file.Path == m.selectedFilePath {
			prefix = "> "
			lineStyle = selectedStyle
		}

		token, tokenStyle := fileToken(file)
		main := truncate(fmt.Sprintf("%s%-8s %s", prefix, token, file.BaseName), innerWidth)
		lines = append(lines, lineStyle.Render(tokenStyle.Render(main)))
		if file.Dir != "." && file.Dir != "" {
			lines = append(lines, mutedStyle.Render(truncate("   "+file.Dir, innerWidth)))
		}
	}

	body := strings.Join(lines[1:], "\n")
	return style.Render(renderViewport(innerWidth, contentHeight, 0, lines[0], body))
}

func (m Model) renderCommits(commits []commitmodel.Commit, width, height int, active bool, stageLabel ...string) string {
	style := basePaneStyle.Width(width)
	if active {
		style = activePaneStyle.Width(width)
	}
	innerWidth := paneInnerWidth(width)
	contentHeight := max(1, height-1)

	title := "COMMITS"
	if len(stageLabel) > 0 {
		title = stageLabel[0]
	}
	lines := []string{headerTextStyle.Render(title)}
	if errText := m.commitErr[m.selectedWorktreePath]; strings.TrimSpace(errText) != "" {
		return style.Render(renderViewport(innerWidth, contentHeight, 0, lines[0], dangerStyle.Render(truncate(errText, innerWidth))))
	}
	if len(commits) == 0 {
		body := strings.Join([]string{
			"No branch commits ahead of base.",
			mutedStyle.Render("Press f to inspect current uncommitted changes."),
		}, "\n")
		return style.Render(renderViewport(innerWidth, contentHeight, 0, lines[0], body))
	}

	maxItems := visibleItemCount(height)
	start, end := windowFromOffset(len(commits), m.commitOffset, maxItems)
	for _, commit := range commits[start:end] {
		prefix := "  "
		lineStyle := lipgloss.NewStyle()
		if commit.Hash == m.selectedCommitHash {
			prefix = "> "
			lineStyle = selectedStyle
		}
		main := truncate(fmt.Sprintf("%s%-8s %s", prefix, commit.ShortHash, commit.Subject), innerWidth)
		lines = append(lines, lineStyle.Render(main))
		lines = append(lines, mutedStyle.Render(truncate("   "+commit.Relative, innerWidth)))
	}

	body := strings.Join(lines[1:], "\n")
	return style.Render(renderViewport(innerWidth, contentHeight, 0, lines[0], body))
}

func (m Model) renderCommitFiles(files []commitmodel.CommitFile, width, height int, active bool, stageLabel ...string) string {
	style := basePaneStyle.Width(width)
	if active {
		style = activePaneStyle.Width(width)
	}
	innerWidth := paneInnerWidth(width)
	contentHeight := max(1, height-1)

	title := "COMMIT FILES"
	if len(stageLabel) > 0 {
		title = stageLabel[0]
	}
	lines := []string{headerTextStyle.Render(title)}
	if errText := m.commitFilesErr[commitFilesKey(m.selectedWorktreePath, m.selectedCommitHash)]; strings.TrimSpace(errText) != "" {
		return style.Render(renderViewport(innerWidth, contentHeight, 0, lines[0], dangerStyle.Render(truncate(errText, innerWidth))))
	}
	if len(files) == 0 {
		body := strings.Join([]string{
			"No files recorded for this commit.",
			mutedStyle.Render("Press h to return to commit list."),
		}, "\n")
		return style.Render(renderViewport(innerWidth, contentHeight, 0, lines[0], body))
	}

	selectedPath := m.selectedCommitFilePath
	if selectedPath == "" && len(files) > 0 {
		selectedPath = files[0].Path
	}
	maxItems := visibleItemCount(height)
	start, end := windowFromOffset(len(files), m.commitFileOffset, maxItems)
	for _, file := range files[start:end] {
		prefix := "  "
		lineStyle := lipgloss.NewStyle()
		if file.Path == selectedPath {
			prefix = "> "
			lineStyle = selectedStyle
		}
		token, tokenStyle := commitFileToken(file.StatusCode)
		main := truncate(fmt.Sprintf("%s%-8s %s", prefix, token, file.BaseName), innerWidth)
		lines = append(lines, lineStyle.Render(tokenStyle.Render(main)))
		if file.Dir != "." && file.Dir != "" {
			lines = append(lines, mutedStyle.Render(truncate("   "+file.Dir, innerWidth)))
		}
	}

	body := strings.Join(lines[1:], "\n")
	return style.Render(renderViewport(innerWidth, contentHeight, 0, lines[0], body))
}

func (m Model) renderDiff(width, height int, active bool, stageLabel ...string) string {
	style := basePaneStyle.Width(width)
	if active {
		style = activePaneStyle.Width(width)
	}
	innerWidth := paneInnerWidth(width)
	contentHeight := max(1, height-1)

	title := "DIFF"
	if len(stageLabel) > 0 {
		title = stageLabel[0] + "  " + title
	}
	lines := []string{headerTextStyle.Render(title)}
	if m.centerMode == centerCommitFiles && m.selectedCommitFilePath == "" {
		return style.Render(renderViewport(innerWidth, contentHeight, 0, lines[0], "Select a commit file to inspect."))
	}
	if m.centerMode == centerCommits && m.selectedCommitHash == "" {
		return style.Render(renderViewport(innerWidth, contentHeight, 0, lines[0], "Select a commit to inspect."))
	}
	if m.centerMode == centerFiles && m.selectedFilePath == "" {
		return style.Render(renderViewport(innerWidth, contentHeight, 0, lines[0], "Select a changed file to inspect."))
	}
	if m.diffErr != "" {
		return style.Render(renderViewport(innerWidth, contentHeight, 0, lines[0], dangerStyle.Render(truncate(m.diffErr, innerWidth))))
	}

	header := m.diff.Header
	if header == "" {
		header = m.selectedFilePath
	}
	if m.diffLoading {
		header += "  [refreshing...]"
	}
	lines = append(lines, truncate(header, innerWidth))

	contentLines := []string{}
	if strings.TrimSpace(m.diff.PatchText) == "" {
		contentLines = append(contentLines, mutedStyle.Render("loading diff..."))
	} else {
		contentLines = strings.Split(strings.TrimRight(m.diff.PatchText, "\n"), "\n")
	}
	contentLines = clampLines(contentLines, max(8, innerWidth))

	viewHeight := max(1, height-4)
	if m.diffScroll > max(0, len(contentLines)-viewHeight) {
		m.diffScroll = max(0, len(contentLines)-viewHeight)
	}
	end := min(len(contentLines), m.diffScroll+viewHeight)
	for _, line := range contentLines[m.diffScroll:end] {
		lines = append(lines, styleDiffLine(line))
	}
	if m.diff.TooLarge {
		lines = append(lines, mutedStyle.Render("Preview truncated - open full diff in pager/editor."))
	}

	body := strings.Join(lines[1:], "\n")
	return style.Render(renderViewport(innerWidth, contentHeight, m.diffScroll, lines[0], body))
}

func renderError(message string) string {
	return dangerStyle.Render("patchdeck failed: " + message)
}

func truncate(value string, width int) string {
	if width <= 3 || len(value) <= width {
		return value
	}
	return value[:width-3] + "..."
}

func clampLines(lines []string, width int) []string {
	if width <= 0 {
		return lines
	}
	clamped := make([]string, 0, len(lines))
	for _, line := range lines {
		clamped = append(clamped, truncate(line, width))
	}
	return clamped
}

func paneInnerWidth(width int) int {
	// Normal border + horizontal padding(1,1) consumes 4 columns.
	return max(4, width-4)
}

func renderViewport(width, height, yOffset int, title, body string) string {
	vp := viewport.New(width, height)
	vp.SetContent(body)
	if yOffset < 0 {
		yOffset = 0
	}
	maxOffset := max(0, vp.TotalLineCount()-height)
	if yOffset > maxOffset {
		yOffset = maxOffset
	}
	vp.YOffset = yOffset
	return title + "\n" + vp.View()
}

func (m Model) paneContentHeight() int {
	if m.width <= 0 || m.height <= 0 {
		return 3
	}

	rows := m.sortedRows()
	header := m.renderHeader(rows)
	footer := m.renderFooter()

	// Keep a 1-line safety margin. Some terminal layouts end up effectively
	// overflowing by one row when the frame exactly matches the viewport.
	bodyHeight := max(7, m.height-lipgloss.Height(header)-lipgloss.Height(footer)-1)
	return max(3, bodyHeight-2)
}

func (m Model) listVisibleItemCount() int {
	return visibleItemCount(m.paneContentHeight())
}

func visibleItemCount(height int) int {
	usable := max(1, height-2)
	count := usable / 2
	if count < 1 {
		count = 1
	}
	return count
}

func windowFromOffset(total, offset, maxItems int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	if maxItems <= 0 || total <= maxItems {
		return 0, total
	}
	if offset < 0 {
		offset = 0
	}
	maxOffset := max(0, total-maxItems)
	if offset > maxOffset {
		offset = maxOffset
	}
	return offset, min(total, offset+maxItems)
}

func followOffset(currentOffset, selected, maxItems int) int {
	if maxItems <= 0 {
		return 0
	}
	if selected < currentOffset {
		return selected
	}
	if selected >= currentOffset+maxItems {
		return selected - maxItems + 1
	}
	return currentOffset
}

func bottomOffset(total, maxItems int) int {
	if total <= maxItems {
		return 0
	}
	return total - maxItems
}

func fileToken(file statusmodel.ChangedFile) (string, lipgloss.Style) {
	switch {
	case file.IsDir:
		return "DIR", pathStyle
	case file.Conflicted:
		return "CONFLICT", dangerStyle
	case file.Untracked:
		return "NEW", addedStyle
	case file.Deleted:
		return "DELETED", removedStyle
	case file.StagedCode == 'M' || file.WorkCode == 'M':
		return "MOD", selectedStyle
	case file.StagedCode == 'A':
		return "ADDED", addedStyle
	case file.StagedCode == 'R':
		return "RENAMED", hunkStyle
	default:
		return strings.TrimSpace(file.StatusCode), mutedStyle
	}
}

func commitFileToken(status string) (string, lipgloss.Style) {
	switch status {
	case "A":
		return "ADDED", addedStyle
	case "D":
		return "DELETED", removedStyle
	case "M":
		return "MOD", selectedStyle
	default:
		return status, mutedStyle
	}
}

func styleDiffLine(line string) string {
	switch {
	case strings.HasPrefix(line, "@@"):
		return hunkStyle.Render(line)
	case strings.HasPrefix(line, "diff --git"),
		strings.HasPrefix(line, "index "),
		strings.HasPrefix(line, "--- "),
		strings.HasPrefix(line, "+++ "):
		return pathStyle.Render(line)
	case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
		return addedStyle.Render(line)
	case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
		return removedStyle.Render(line)
	default:
		return line
	}
}

func (m Model) diffPageStep() int {
	viewHeight := max(1, m.height-8)
	if viewHeight < 4 {
		return 1
	}
	return viewHeight / 2
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
