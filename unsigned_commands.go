package main

import (
	"strings"
	"path/filepath"
	"fmt"
	"os"
)

func getUploadCommand(command string) (string, bool) {
	// buildkite-signed-pipeline upload
	rawUploadCommand := fmt.Sprintf("%s upload", filepath.Base(os.Args[0]))
	if strings.HasPrefix(command, rawUploadCommand) {
		return rawUploadCommand, true
	}

	vanillaUploadCommand := "buildkite-agent pipeline upload"
	if strings.HasPrefix(command, vanillaUploadCommand) {
		return vanillaUploadCommand, true
	}

	return "", false
}

func IsUnsignedCommandOk(command string) (bool, error) {
	uploadCommand, ok := getUploadCommand(command)
	if !ok {
		return false, nil
	}

	// straight upload
	if uploadCommand == command {
		return true, nil
	}

	uploadPrefix := uploadCommand + " "
	// If there's no additional arguments, bail early
	if !strings.HasPrefix(command, uploadPrefix) {
		return false, nil
	}

	fileArgument := strings.TrimPrefix(command, uploadPrefix)
	isLocal, err := isWorkingDirectoryFile(fileArgument)

	if err != nil {
		return false, err
	}

	return isLocal, nil
}

func isWorkingDirectoryFile(fileName string) (bool, error) {
	// this is based on https://github.com/buildkite/agent/blob/cc07aba854e35f0f31b0d743d7ec2829b425bb6a/bootstrap/bootstrap.go#L1013-L1030
	// and ensures the given filename exists in the working directory
	workingDirectory, err := os.Getwd()
	if err != nil {
		return false, err
	}
	pathToFile, err := filepath.Abs(filepath.Join(workingDirectory, fileName))
	if err != nil {
		return false, err
	}

	// Make sure the file is definitely within this working directory
	return fileExists(pathToFile) &&
		strings.HasPrefix(pathToFile, workingDirectory + string(os.PathSeparator)), nil
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}
