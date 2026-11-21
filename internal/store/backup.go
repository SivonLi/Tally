package store

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// BackupInfo 描述一个本地备份。
type BackupInfo struct {
	Name      string
	Path      string
	CreatedAt time.Time
	Size      int64
}

const backupLimit = 20

// BackupBeforeOpen 在打开数据库前复制现有数据库与原始文件，保留最近二十份备份。
func BackupBeforeOpen(root string) (string, error) {
	if root == "" || root == "." {
		return "", nil
	}
	database := filepath.Join(root, "tally.db")
	info, err := os.Stat(database)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if info.Size() == 0 {
		return "", nil
	}
	return CreateBackup(root)
}

// CreateBackup 复制数据库与原始文件目录，返回备份目录。
func CreateBackup(root string) (string, error) {
	database := filepath.Join(root, "tally.db")
	info, err := os.Stat(database)
	if err != nil {
		return "", err
	}
	if info.Size() == 0 {
		return "", fmt.Errorf("数据库文件为空")
	}
	name := time.Now().UTC().Format("20060102-150405.000000000")
	dir := filepath.Join(root, "backups", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := copyFile(database, filepath.Join(dir, "tally.db"), info.Mode().Perm()); err != nil {
		return "", err
	}
	files := filepath.Join(root, "files")
	if stat, err := os.Stat(files); err == nil && stat.IsDir() {
		if err := copyDir(files, filepath.Join(dir, "files")); err != nil {
			return "", err
		}
	}
	if err := pruneBackups(root); err != nil {
		return "", err
	}
	return dir, nil
}

// ListBackups 返回从新到旧的备份。
func ListBackups(root string) ([]BackupInfo, error) {
	entries, err := os.ReadDir(filepath.Join(root, "backups"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	backups := make([]BackupInfo, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, "backups", entry.Name())
		info, err := os.Stat(filepath.Join(dir, "tally.db"))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		created := info.ModTime()
		if parsed, err := time.Parse("20060102-150405.000000000", entry.Name()); err == nil {
			created = parsed.UTC()
		}
		backups = append(backups, BackupInfo{
			Name:      entry.Name(),
			Path:      dir,
			CreatedAt: created,
			Size:      info.Size(),
		})
	}
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})
	return backups, nil
}

// RestoreBackup 用备份替换当前数据库与原始文件。调用前必须关闭数据库连接。
func RestoreBackup(root, name string) error {
	return restoreBackup(root, name, true)
}

func restoreBackup(root, name string, keepCurrent bool) error {
	if name == "" || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("备份名称不正确")
	}
	dir := filepath.Join(root, "backups", name)
	source := filepath.Join(dir, "tally.db")
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	current := filepath.Join(root, "tally.db")
	if keepCurrent {
		if _, err := os.Stat(current); err == nil {
			if _, err := CreateBackup(root); err != nil {
				return err
			}
		}
	}
	temp := current + ".restore"
	if err := copyFile(source, temp, info.Mode().Perm()); err != nil {
		return err
	}
	if err := os.Remove(current); err != nil && !errors.Is(err, fs.ErrNotExist) {
		os.Remove(temp)
		return err
	}
	if err := os.Rename(temp, current); err != nil {
		return err
	}
	files := filepath.Join(root, "files")
	if stat, err := os.Stat(filepath.Join(dir, "files")); err == nil && stat.IsDir() {
		if err := os.RemoveAll(files); err != nil {
			return err
		}
		if err := copyDir(filepath.Join(dir, "files"), files); err != nil {
			return err
		}
	}
	return nil
}

func pruneBackups(root string) error {
	backups, err := ListBackups(root)
	if err != nil {
		return err
	}
	for index := backupLimit; index < len(backups); index++ {
		if err := os.RemoveAll(backups[index].Path); err != nil {
			return err
		}
	}
	return nil
}

func copyDir(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return copyFile(path, destination, info.Mode().Perm())
	})
}

func copyFile(source, target string, mode fs.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		return err
	}
	if err := output.Sync(); err != nil {
		output.Close()
		return err
	}
	return output.Close()
}
