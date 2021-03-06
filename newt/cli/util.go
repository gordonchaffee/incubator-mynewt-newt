/**
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package cli

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/hashicorp/logutils"
	"github.com/spf13/viper"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"
)

type NewtError struct {
	Text       string
	StackTrace []byte
}

var Logger *log.Logger
var Verbosity int
var OK_STRING = " ok!\n"

const (
	VERBOSITY_SILENT  = 0
	VERBOSITY_QUIET   = 1
	VERBOSITY_DEFAULT = 2
	VERBOSITY_VERBOSE = 3
)

func (se *NewtError) Error() string {
	return se.Text + "\n" + string(se.StackTrace)
}

func NewNewtError(msg string) *NewtError {
	err := &NewtError{
		Text:       msg,
		StackTrace: make([]byte, 1<<16),
	}

	runtime.Stack(err.StackTrace, true)

	return err
}

func NewtErrorNoTrace(msg string) *NewtError {
	return &NewtError{
		Text:       msg,
		StackTrace: nil,
	}
}

// Initialize the CLI module
func Init(level string, silent bool, quiet bool, verbose bool) {
	if level == "" {
		level = "WARN"
	}

	filter := &logutils.LevelFilter{
		Levels: []logutils.LogLevel{"DEBUG", "VERBOSE", "INFO",
			"WARN", "ERROR"},
		MinLevel: logutils.LogLevel(level),
		Writer:   os.Stderr,
	}

	log.SetOutput(filter)

	if silent {
		Verbosity = VERBOSITY_SILENT
	} else if quiet {
		Verbosity = VERBOSITY_QUIET
	} else if verbose {
		Verbosity = VERBOSITY_VERBOSE
	} else {
		Verbosity = VERBOSITY_DEFAULT
	}
}

func checkBoolMap(mapVar map[string]bool, item string) bool {
	v, ok := mapVar[item]
	return v && ok
}

// Read in the configuration file specified by name, in path
// return a new viper config object if successful, and error if not
func ReadConfig(path string, name string) (*viper.Viper, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigName(name)
	v.AddConfigPath(path)

	err := v.ReadInConfig()
	if err != nil {
		return nil, NewNewtError(err.Error())
	} else {
		return v, nil
	}
}

func GetStringIdentities(v *viper.Viper, idents map[string]string, key string) string {
	val := v.GetString(key)

	// Process the identities in alphabetical order to ensure consistent
	// results across repeated runs.
	var identKeys []string
	for ident, _ := range idents {
		identKeys = append(identKeys, ident)
	}
	sort.Strings(identKeys)

	for _, ident := range identKeys {
		overwriteVal := v.GetString(key + "." + ident + ".OVERWRITE")
		if overwriteVal != "" {
			val = strings.Trim(overwriteVal, "\n")
			break
		}

		appendVal := v.GetString(key + "." + ident)
		if appendVal != "" {
			val += " " + strings.Trim(appendVal, "\n")
		}
	}
	return strings.TrimSpace(val)
}

func GetStringSliceIdentities(v *viper.Viper, idents map[string]string,
	key string) []string {

	val := v.GetStringSlice(key)

	// string empty items
	result := []string{}
	for _, item := range val {
		if item == "" || item == " " {
			continue
		}
		result = append(result, item)
	}

	for item, _ := range idents {
		result = append(result, v.GetStringSlice(key+"."+item)...)
	}

	return result
}

func NodeExist(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	} else {
		return false
	}
}

// Check whether the node (either dir or file) specified by path exists
func NodeNotExist(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return true
	} else {
		return false
	}
}

func FileModificationTime(path string) (time.Time, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		epoch := time.Unix(0, 0)
		if os.IsNotExist(err) {
			return epoch, nil
		} else {
			return epoch, NewNewtError(err.Error())
		}
	}

	return fileInfo.ModTime(), nil
}

// Execute the command specified by cmdStr on the shell and return results
func ShellCommand(cmdStr string) ([]byte, error) {
	log.Print("[VERBOSE] " + cmdStr)
	cmd := exec.Command("sh", "-c", cmdStr)

	o, err := cmd.CombinedOutput()
	log.Print("[VERBOSE] o=" + string(o))
	if err != nil {
		return o, NewNewtError(err.Error())
	} else {
		return o, nil
	}
}

// Run interactive shell command
func ShellInteractiveCommand(cmdStr []string) error {
	log.Print("[VERBOSE] " + cmdStr[0])

	//
	// Block SIGINT, at least.
	// Otherwise Ctrl-C meant for gdb would kill newt.
	//
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)
	go func() {
		<-c
	}()

	// Transfer stdin, stdout, and stderr to the new process
	// and also set target directory for the shell to start in.
	pa := os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	}

	// Start up a new shell.
	proc, err := os.StartProcess(cmdStr[0], cmdStr, &pa)
	if err != nil {
		signal.Stop(c)
		return NewNewtError(err.Error())
	}

	// Release and exit
	_, err = proc.Wait()
	if err != nil {
		signal.Stop(c)
		return NewNewtError(err.Error())
	}
	signal.Stop(c)
	return nil
}

func CopyFile(srcFile string, destFile string) error {
	_, err := ShellCommand(fmt.Sprintf("mkdir -p %s", filepath.Dir(destFile)))
	if err != nil {
		return err
	}
	if _, err := ShellCommand(fmt.Sprintf("cp -Rf %s %s", srcFile,
		destFile)); err != nil {
		return err
	}
	return nil
}

func CopyDir(srcDir, destDir string) error {
	return CopyFile(srcDir, destDir)
}

// Print Silent, Quiet and Verbose aware status messages
func StatusMessage(level int, message string, args ...interface{}) {
	if Verbosity >= level {
		fmt.Printf(message, args...)
	}
}

// Reads each line from the specified text file into an array of strings.  If a
// line ends with a backslash, it is concatenated with the following line.
func ReadLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, NewNewtError(err.Error())
	}
	defer file.Close()

	lines := []string{}
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		concatted := false

		if len(lines) != 0 {
			prevLine := lines[len(lines)-1]
			if len(prevLine) > 0 && prevLine[len(prevLine)-1:] == "\\" {
				prevLine = prevLine[:len(prevLine)-1]
				prevLine += line
				lines[len(lines)-1] = prevLine

				concatted = true
			}
		}

		if !concatted {
			lines = append(lines, line)
		}
	}

	if scanner.Err() != nil {
		return lines, NewNewtError(scanner.Err().Error())
	}

	return lines, nil
}

// Determines if a file was previously built with a command line invocation
// different from the one specified.
//
// @param dstFile               The output file whose build invocation is being
//                                  tested.
// @param cmd                   The command that would be used to generate the
//                                  specified destination file.
//
// @return                      true if the command has changed or if the
//                                  destination file was never built;
//                              false otherwise.
func CommandHasChanged(dstFile string, cmd string) bool {
	cmdFile := dstFile + ".cmd"
	prevCmd, err := ioutil.ReadFile(cmdFile)
	if err != nil {
		return true
	}

	return bytes.Compare(prevCmd, []byte(cmd)) != 0
}

// Writes a file containing the command-line invocation used to generate the
// specified file.  The file that this function writes can be used later to
// determine if the set of compiler options has changed.
//
// @param dstFile               The output file whose build invocation is being
//                                  recorded.
// @param cmd                   The command to write.
func WriteCommandFile(dstFile string, cmd string) error {
	cmdPath := dstFile + ".cmd"
	err := ioutil.WriteFile(cmdPath, []byte(cmd), 0644)
	if err != nil {
		return err
	}

	return nil
}

// Removes all duplicate strings from the specified array, while preserving
// order.
func UniqueStrings(elems []string) []string {
	set := make(map[string]bool)
	result := make([]string, 0)

	for _, elem := range elems {
		if !set[elem] {
			result = append(result, elem)
			set[elem] = true
		}
	}

	return result
}
