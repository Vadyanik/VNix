package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func configureGitIdentity(config Config) error {
	for _, setting := range []struct{ key, value string }{
		{"user.name", config.GitUserName},
		{"user.email", config.GitUserEmail},
	} {
		if strings.TrimSpace(setting.value) != "" {
			if err := runCommand("git", "config", "--local", setting.key, setting.value); err != nil {
				return err
			}
		}
	}
	return nil
}

func stageRebuildFiles(config Config) error {
	if config.GitAddAll {
		return runCommand("git", "add", "--all", "--", ".")
	}
	path, err := managedPackagesFile()
	if err != nil {
		return err
	}
	return runCommand("git", "add", "--", path)
}

func gitHasStagedChanges() (bool, error) {
	err := exec.Command("git", "diff", "--cached", "--quiet", "--exit-code").Run()
	if err == nil {
		return false, nil
	}
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
		return true, nil
	}
	return false, err
}

func configuredCommitMessage(config Config) string {
	if !defaultEnabled(config.AICommitMessage) {
		return fallbackCommitMessage(config.CommitMessagePrefix)
	}
	if command := strings.TrimSpace(config.CommitMessageCommand); command != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "bash", "-c", command)
		cmd.Stderr = os.Stderr
		output, err := cmd.Output()
		if err == nil {
			if message, cleanErr := cleanCommitMessage(string(output)); cleanErr == nil {
				return message
			}
		}
		fmt.Fprintln(os.Stderr, "Commit message command failed or returned no message; using timestamp fallback.")
		return fallbackCommitMessage(config.CommitMessagePrefix)
	}
	return aiCommitMessage(config.CommitMessagePrefix)
}

func pushRebuildChanges(config Config) error {
	if config.GitPushBranch != "" && config.GitPushRemote == "" {
		return fmt.Errorf("git_push_branch requires git_push_remote")
	}
	args := []string{"push"}
	if config.GitPushForce {
		args = append(args, "--force-with-lease")
	}
	if config.GitPushRemote != "" {
		args = append(args, "--", config.GitPushRemote)
		if config.GitPushBranch != "" {
			args = append(args, config.GitPushBranch)
		}
	}
	fmt.Println("$ git", strings.Join(args, " "))
	cmd := exec.Command("git", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if config.GitSSHCommand != "" {
		cmd.Env = append(os.Environ(), "GIT_SSH_COMMAND="+config.GitSSHCommand)
	}
	return cmd.Run()
}

func runConfiguredHooks(config Config, name string, hooks []string) error {
	if !defaultEnabled(config.HooksEnabled) {
		return nil
	}
	for _, hook := range hooks {
		if err := runHooks(name, []string{hook}); err != nil {
			if !config.HooksContinueOnError {
				return fmt.Errorf("%s hook failed: %w", name, err)
			}
			fmt.Fprintf(os.Stderr, "Warning: %s hook failed: %v; continuing.\n", name, err)
		}
	}
	return nil
}

// Include untracked files in both the review and the pending diff metrics.
func pendingGitDiff(options ...string) ([]byte, error) {
	args := append([]string{"diff", "--no-ext-diff", "--no-renames", "--ignore-submodules=dirty"}, options...)
	output, err := exec.Command("git", append(args, "HEAD", "--")...).Output()
	if err != nil {
		return nil, err
	}
	untracked, err := exec.Command("git", "ls-files", "--others", "--exclude-standard", "-z").Output()
	if err != nil {
		return nil, err
	}
	for _, path := range strings.Split(string(untracked), "\x00") {
		if path == "" {
			continue
		}
		args := append([]string{"diff", "--no-index", "--no-ext-diff"}, options...)
		diff, err := exec.Command("git", append(args, "--", os.DevNull, path)...).Output()
		if err != nil {
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
				return nil, err
			}
		}
		for _, option := range options {
			if option == "--numstat" {
				fields := strings.SplitN(strings.TrimSuffix(string(diff), "\n"), "\t", 3)
				if len(fields) == 3 {
					diff = []byte(fields[0] + "\t" + fields[1] + "\t" + path + "\n")
				}
			}
		}
		output = append(output, diff...)
	}
	return output, nil
}
