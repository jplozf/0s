// **********************************************************************
// Copyright (C) 2025 J.P. Liguori (jpl@ozf.fr)
// **********************************************************************
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/melbahja/goph"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type Config struct {
	Current      string                `json:"current"`
	Repositories map[string]Repository `json:"repositories"`
}

type Repository struct {
	Type       string `json:"type"`
	Path       string `json:"path,omitempty"`
	Host       string `json:"host,omitempty"`
	Port       uint   `json:"port,omitempty"`
	User       string `json:"user,omitempty"`
	PrivateKey string `json:"private_key,omitempty"`
	Password   string `json:"password,omitempty"`
}

func main() {
	// Load configuration
	config, err := loadConfig()
	if err != nil {
		fmt.Println("Error loading configuration:", err)
		os.Exit(1)
	}

	// Get command-line arguments
	args := os.Args[1:]

	// Check for commands
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		listRepositories(config)
	case "set":
		if len(args) < 2 {
			fmt.Println("Please specify a repository to set.")
			os.Exit(1)
		}
		setRepository(config, args[1])
	case "show":
		showRepository(config)
	case "get":
		if len(args) < 2 {
			fmt.Println("Please specify a file or folder to get.")
			os.Exit(1)
		}
		getRepository(config, args[1])
	case "put":
		if len(args) < 2 {
			fmt.Println("Please specify a file or folder to put.")
			os.Exit(1)
		}
		putRepository(config, args[1])
	case "cd":
		if len(args) < 2 {
			fmt.Println("Please specify a directory to change to.")
			os.Exit(1)
		}
		changeDirectory(config, args[1])
	default:
		printUsage()
	}
}

func loadConfig() (*Config, error) {
	// Open config file
	configFile, err := os.Open("config/config.json")
	if err != nil {
		return nil, err
	}
	defer configFile.Close()

	// Read config file
	byteValue, err := ioutil.ReadAll(configFile)
	if err != nil {
		return nil, err
	}

	// Unmarshal JSON
	var config Config
	json.Unmarshal(byteValue, &config)

	return &config, nil
}

func saveConfig(config *Config) error {
	// Marshal JSON
	byteValue, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	// Write config file
	err = ioutil.WriteFile("config/config.json", byteValue, 0644)
	if err != nil {
		return err
	}

	return nil
}

func listRepositories(config *Config) {
	fmt.Println("Available repositories:")
	for name, repo := range config.Repositories {
		if name == config.Current {
			fmt.Printf(" * %s (%s, %s)\n", name, repo.Type, repo.Path)
		} else {
			fmt.Printf("   %s (%s, %s)\n", name, repo.Type, repo.Path)
		}
	}
}

func setRepository(config *Config, name string) {
	// Check if repository exists
	if _, ok := config.Repositories[name]; !ok {
		fmt.Printf("Repository '%s' not found.\n", name)
		os.Exit(1)
	}

	// Set current repository
	config.Current = name

	// Save config
	err := saveConfig(config)
	if err != nil {
		fmt.Println("Error saving configuration:", err)
		os.Exit(1)
	}

	fmt.Printf("Current repository set to '%s'.\n", name)
}

func showRepository(config *Config) {
	// Get current repository
	repo := config.Repositories[config.Current]

	// Check repository type
	switch repo.Type {
	case "local", "network":
		// List files and folders
		files, err := ioutil.ReadDir(repo.Path)
		if err != nil {
			fmt.Println("Error reading repository:", err)
			os.Exit(1)
		}

		for _, file := range files {
			if file.IsDir() {
				fmt.Printf("%s/\n", file.Name())
			} else {
				fmt.Println(file.Name())
			}
		}
	case "ssh":
		// Get SSH client
		client, err := getSSHClient(&repo)
		if err != nil {
			fmt.Println("Error connecting to SSH server:", err)
			os.Exit(1)
		}
		defer client.Close()

		// Get SFTP client
		sftp, err := client.NewSftp()
		if err != nil {
			fmt.Println("Error creating SFTP client:", err)
			os.Exit(1)
		}
		defer sftp.Close()

		// List files
		files, err := sftp.ReadDir(repo.Path)
		if err != nil {
			fmt.Println("Error reading remote directory:", err)
			os.Exit(1)
		}

		for _, file := range files {
			if file.IsDir() {
				fmt.Printf("%s/\n", file.Name())
			} else {
				fmt.Println(file.Name())
			}
		}

	default:
		fmt.Printf("Repository type '%s' not implemented yet.\n", repo.Type)
	}
}

func getRepository(config *Config, name string) {
	// Get current repository
	repo := config.Repositories[config.Current]

	switch repo.Type {
	case "local", "network":
		// Get source and destination paths
		srcPath := filepath.Join(repo.Path, name)
		destPath, err := os.Getwd()
		if err != nil {
			fmt.Println("Error getting current directory:", err)
			os.Exit(1)
		}
		destPath = filepath.Join(destPath, name)

		// Copy file or folder
		err = copy(srcPath, destPath)
		if err != nil {
			fmt.Println("Error getting file or folder:", err)
			os.Exit(1)
		}
	case "ssh":
		// Get SSH client
		client, err := getSSHClient(&repo)
		if err != nil {
			fmt.Println("Error connecting to SSH server:", err)
			os.Exit(1)
		}
		defer client.Close()

		sftp, err := client.NewSftp()
		if err != nil {
			fmt.Println("Error creating SFTP client:", err)
			os.Exit(1)
		}
		defer sftp.Close()

		// Get remote and local paths
		remotePath := filepath.ToSlash(filepath.Join(repo.Path, name))
		localPath, err := os.Getwd()
		if err != nil {
			fmt.Println("Error getting current directory:", err)
			os.Exit(1)
		}
		localPath = filepath.Join(localPath, name)

		// Check if remote path is a directory or a file
		remoteStat, err := sftp.Stat(remotePath)
		if err != nil {
			fmt.Printf("Error getting remote file info: %v\n", err)
			os.Exit(1)
		}

		if remoteStat.IsDir() {
			err = downloadDirectory(sftp, remotePath, localPath)
		} else {
			err = downloadFile(sftp, remotePath, localPath)
		}

		if err != nil {
			fmt.Printf("Error during 'get' operation: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Printf("Repository type '%s' not implemented yet for 'get'.\n", repo.Type)
		return
	}
}

func putRepository(config *Config, name string) {
	// Get current repository
	repo := config.Repositories[config.Current]

	switch repo.Type {
	case "local", "network":
		// Get source and destination paths
		srcPath, err := filepath.Abs(name)
		if err != nil {
			fmt.Println("Error getting absolute path:", err)
			os.Exit(1)
		}
		destPath := filepath.Join(repo.Path, name)

		// Copy file or folder
		err = copy(srcPath, destPath)
		if err != nil {
			fmt.Println("Error putting file or folder:", err)
			os.Exit(1)
		}
	case "ssh":
		// Get SSH client
		client, err := getSSHClient(&repo)
		if err != nil {
			fmt.Println("Error connecting to SSH server:", err)
			os.Exit(1)
		}
		defer client.Close()

		// Get local and remote paths
		localPath, err := filepath.Abs(name)
		if err != nil {
			fmt.Println("Error getting absolute path:", err)
			os.Exit(1)
		}
		remotePath := filepath.ToSlash(filepath.Join(repo.Path, name))

		// Upload file
		err = client.Upload(localPath, remotePath)
		if err != nil {
			fmt.Println("Error uploading file:", err)
			os.Exit(1)
		}
	default:
		fmt.Printf("Repository type '%s' not implemented yet for 'put'.\n", repo.Type)
		return
	}
}

func copy(src, dest string) error {
	// Get source info
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	// Check if source is a directory
	if srcInfo.IsDir() {
		// Create destination directory
		err = os.MkdirAll(dest, srcInfo.Mode())
		if err != nil {
			return err
		}

		// Get directory contents
		files, err := ioutil.ReadDir(src)
		if err != nil {
			return err
		}

		// Copy directory contents
		for _, file := range files {
			srcPath := filepath.Join(src, file.Name())
			destPath := filepath.Join(dest, file.Name())
			err = copy(srcPath, destPath)
			if err != nil {
				return err
			}
		}
	} else {
		// Open source file
		srcFile, err := os.Open(src)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		// Create destination file
		destFile, err := os.Create(dest)
		if err != nil {
			return err
		}
		defer destFile.Close()

		// Copy file contents
		_, err = io.Copy(destFile, srcFile)
		if err != nil {
			return err
		}
	}

	return nil
}

func printUsage() {
	fmt.Println("Usage: 0s <command>")
	fmt.Println("Commands:")
	fmt.Println("  list       - List all available repositories")
	fmt.Println("  set <repo> - Set the current repository")
	fmt.Println("  show       - Show files in the current repository")
	fmt.Println("  cd <dir>   - Change the current directory for the repository")
	fmt.Println("  get <name> - Get a file or folder from the current repository")
	fmt.Println("  put <name> - Put a file or folder in the current repository")
}

func getSSHClient(repo *Repository) (*goph.Client, error) {
	var auth goph.Auth
	var err error

	// Use password auth if provided, otherwise use public key auth.
	if repo.Password != "" {
		auth = goph.Password(repo.Password)
	} else {
		auth, err = goph.Key(repo.PrivateKey, "")
		if err != nil {
			return nil, err
		}
	}

	// Create new SSH client
	client, err := goph.NewConn(&goph.Config{
		User:     repo.User,
		Addr:     repo.Host,
		Port:     repo.Port,
		Auth:     auth,
		Callback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		return nil, err
	}

	return client, nil
}

func downloadFile(sftp *sftp.Client, remotePath, localPath string) error {
	// Open remote file
	remoteFile, err := sftp.Open(remotePath)
	if err != nil {
		return fmt.Errorf("could not open remote file: %v", err)
	}
	defer remoteFile.Close()

	// Create local file
	localFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("could not create local file: %v", err)
	}
	defer localFile.Close()

	// Copy contents
	_, err = io.Copy(localFile, remoteFile)
	if err != nil {
		return fmt.Errorf("could not copy file contents: %v", err)
	}

	fmt.Printf("Downloaded file '%s'\n", remotePath)
	return nil
}

func downloadDirectory(sftp *sftp.Client, remotePath, localPath string) error {
	// Create local directory
	err := os.MkdirAll(localPath, os.ModePerm)
	if err != nil {
		return fmt.Errorf("could not create local directory: %v", err)
	}
	fmt.Printf("Created directory '%s'\n", localPath)

	// List remote directory contents
	walker := sftp.Walk(remotePath)
	for walker.Step() {
		if walker.Err() != nil {
			return fmt.Errorf("error walking remote directory: %v", walker.Err())
		}

		relPath := walker.Path()[len(remotePath):]
		if relPath == "" {
			continue
		}

		localItemPath := filepath.Join(localPath, relPath)
		remoteItemPath := walker.Path()

		if walker.Stat().IsDir() {
			err = os.MkdirAll(localItemPath, os.ModePerm)
			if err != nil {
				return fmt.Errorf("could not create local subdirectory: %v", err)
			}
			fmt.Printf("Created directory '%s'\n", localItemPath)
		} else {
			err = downloadFile(sftp, remoteItemPath, localItemPath)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func changeDirectory(config *Config, newDir string) {
	repo := config.Repositories[config.Current]

	switch repo.Type {
	case "local", "network":
		newPath := filepath.Join(repo.Path, newDir)

		// Check if the new path exists and is a directory
		info, err := os.Stat(newPath)
		if err != nil {
			fmt.Printf("Error accessing path '%s': %v\n", newPath, err)
			os.Exit(1)
		}
		if !info.IsDir() {
			fmt.Printf("Error: '%s' is not a directory.\n", newPath)
			os.Exit(1)
		}

		repo.Path = newPath

	case "ssh":
		if repo.Path == "" {
			repo.Path = "."
		}

		client, err := getSSHClient(&repo)
		if err != nil {
			fmt.Println("Error connecting to SSH server:", err)
			os.Exit(1)
		}
		defer client.Close()

		sftp, err := client.NewSftp()
		if err != nil {
			fmt.Println("Error creating SFTP client:", err)
			os.Exit(1)
		}
		defer sftp.Close()

		// sftp.Join is not available, must use path package
		newPath := filepath.ToSlash(filepath.Join(repo.Path, newDir))

		// Check if remote path exists and is a directory
		info, err := sftp.Stat(newPath)
		if err != nil {
			fmt.Printf("Error accessing remote path '%s': %v\n", newPath, err)
			os.Exit(1)
		}
		if !info.IsDir() {
			fmt.Printf("Error: remote path '%s' is not a directory.\n", newPath)
			os.Exit(1)
		}

		repo.Path = newPath

	default:
		fmt.Printf("Repository type '%s' not implemented yet for 'cd'.\n", repo.Type)
		return
	}

	config.Repositories[config.Current] = repo
	err := saveConfig(config)
	if err != nil {
		fmt.Println("Error saving configuration:", err)
		os.Exit(1)
	}

	fmt.Printf("Changed directory to '%s'\n", repo.Path)
}
