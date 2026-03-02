package sync

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const cronMarker = "# sb-sync-job"

// cronSchedule converts a duration to a cron schedule expression.
func cronSchedule(interval time.Duration) string {
	minutes := int(interval.Minutes())
	if minutes <= 0 {
		minutes = 30
	}
	if minutes < 60 {
		return fmt.Sprintf("*/%d * * * *", minutes)
	}
	hours := minutes / 60
	if hours < 24 {
		return fmt.Sprintf("0 */%d * * *", hours)
	}
	return fmt.Sprintf("0 0 */%d * *", hours/24)
}

// Enable adds a crontab entry for `sb sync run`.
func Enable(interval time.Duration) error {
	sbPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	// Remove existing entry first
	if err := Disable(); err != nil {
		return err
	}

	existing, err := currentCrontab()
	if err != nil {
		return err
	}

	schedule := cronSchedule(interval)
	entry := fmt.Sprintf("%s %s sync run >> ~/.second-brain/sync.log 2>&1 %s", schedule, sbPath, cronMarker)

	newCrontab := existing
	if newCrontab != "" && !strings.HasSuffix(newCrontab, "\n") {
		newCrontab += "\n"
	}
	newCrontab += entry + "\n"

	return writeCrontab(newCrontab)
}

// Disable removes the sb sync crontab entry.
func Disable() error {
	existing, err := currentCrontab()
	if err != nil {
		return err
	}
	if existing == "" {
		return nil
	}

	lines := strings.Split(existing, "\n")
	var filtered []string
	for _, line := range lines {
		if !strings.Contains(line, cronMarker) {
			filtered = append(filtered, line)
		}
	}

	return writeCrontab(strings.Join(filtered, "\n"))
}

// IsEnabled reports whether the cron job is active and returns the schedule.
func IsEnabled() (bool, string, error) {
	existing, err := currentCrontab()
	if err != nil {
		return false, "", err
	}

	re := regexp.MustCompile(`^(.+?)\s+\S+\s+sync\s+run\b.*` + regexp.QuoteMeta(cronMarker))
	for _, line := range strings.Split(existing, "\n") {
		if matches := re.FindStringSubmatch(line); matches != nil {
			return true, matches[1], nil
		}
	}
	return false, "", nil
}

func currentCrontab() (string, error) {
	out, err := exec.Command("crontab", "-l").Output()
	if err != nil {
		// crontab -l returns error if no crontab exists
		return "", nil
	}
	return string(out), nil
}

func writeCrontab(content string) error {
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(content)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("write crontab: %w (%s)", err, string(out))
	}
	return nil
}
