package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/vsariola/sointu"
	"github.com/vsariola/sointu/compiler"
)

func filterExtensions(input map[string]string, extensions []string) map[string]string {
	ret := map[string]string{}
	for _, ext := range extensions {
		extWithDot := "." + ext
		if inputVal, ok := input[extWithDot]; ok {
			ret[extWithDot] = inputVal
		}
	}
	return ret
}

func main() {
	safe := flag.Bool("n", false, "Never overwrite files; if file already exists and would be overwritten, give an error.")
	list := flag.Bool("l", false, "Do not write files; just list files that would change instead.")
	stdout := flag.Bool("s", false, "Do not write files; write to standard output instead.")
	help := flag.Bool("h", false, "Show help.")
	library := flag.Bool("a", false, "Compile Sointu into a library. Input files are not needed.")
	jsonOut := flag.Bool("j", false, "Output the song as .json file instead of compiling.")
	yamlOut := flag.Bool("y", false, "Output the song as .yml file instead of compiling.")
	tmplDir := flag.String("t", "", "When compiling, use the templates in this directory instead of the standard templates.")
	directory := flag.String("o", "", "Directory where to output all files. The directory and its parents are created if needed. By default, everything is placed in the same directory where the original song file is.")
	extensionsOut := flag.String("e", "", "Output only the compiled files with these comma separated extensions. For example: h,asm")
	hold := flag.Int("hold", -1, "New value to be used as the hold value. -1 = do not change.")
	targetArch := flag.String("arch", runtime.GOARCH, "Target architecture. Defaults to OS architecture. Possible values: 386, amd64")
	targetOs := flag.String("os", runtime.GOOS, "Target OS. Defaults to current OS. Possible values: windows, darwin, linux. Anything else is assumed linuxy.")
	flag.Usage = printUsage
	flag.Parse()
	if (flag.NArg() == 0 && !*library) || *help {
		flag.Usage()
		os.Exit(0)
	}
	compile := !*jsonOut && !*yamlOut // if the user gives nothing to output, then the default behaviour is to compile the file
	var comp *compiler.Compiler
	if compile || *library {
		var err error
		if *tmplDir != "" {
			comp, err = compiler.NewFromTemplates(*targetOs, *targetArch, *tmplDir)
		} else {
			comp, err = compiler.New(*targetOs, *targetArch)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, `error creating compiler: %v`, err)
			os.Exit(1)
		}
	}
	output := func(filename string, extension string, contents []byte) error {
		if *stdout {
			fmt.Print(string(contents))
			return nil
		}
		dir, name := filepath.Split(filename)
		if *directory != "" {
			dir = *directory
		}
		name = strings.TrimSuffix(name, filepath.Ext(name)) + extension
		f := filepath.Join(dir, name)
		original, err := ioutil.ReadFile(f)
		if err == nil {
			if bytes.Compare(original, contents) == 0 {
				return nil // no need to update
			}
			if !*list && *safe {
				return fmt.Errorf("file %v would be overwritten by compiler", f)
			}
		}
		if *list {
			fmt.Println(f)
		} else {
			if dir != "" {
				if err := os.MkdirAll(dir, os.ModePerm); err != nil {
					return fmt.Errorf("could not create output directory %v: %v", dir, err)
				}
			}
			err := ioutil.WriteFile(f, contents, 0644)
			if err != nil {
				return fmt.Errorf("could not write file %v: %v", f, err)
			}
		}
		return nil
	}
	process := func(filename string) error {
		inputBytes, err := ioutil.ReadFile(filename)
		if err != nil {
			return fmt.Errorf("could not read file %v: %v", filename, err)
		}
		var song sointu.Song
		if errJSON := json.Unmarshal(inputBytes, &song); errJSON != nil {
			if errYaml := yaml.Unmarshal(inputBytes, &song); errYaml != nil {
				return fmt.Errorf("song could not be unmarshaled as a .json (%v) or .yml (%v)", errJSON, errYaml)
			}
		}
		if *hold > -1 {
			err = song.UpdateHold(byte(*hold))
			if err != nil {
				return fmt.Errorf("error updating the hold value of the song: %v", err)
			}
		}
		var compiledPlayer map[string]string
		if compile {
			var err error
			compiledPlayer, err = comp.Song(&song)
			if err != nil {
				return fmt.Errorf("compiling player failed: %v", err)
			}
			if len(*extensionsOut) > 0 {
				compiledPlayer = filterExtensions(compiledPlayer, strings.Split(*extensionsOut, ","))
			}
			for extension, code := range compiledPlayer {
				if err := output(filename, extension, []byte(code)); err != nil {
					return fmt.Errorf("error outputting %v file: %v", extension, err)
				}
			}
		}
		if *jsonOut {
			jsonSong, err := json.Marshal(song)
			if err != nil {
				return fmt.Errorf("could not marshal the song as json file: %v", err)
			}
			if err := output(filename, ".json", jsonSong); err != nil {
				return fmt.Errorf("error outputting json file: %v", err)
			}
		}
		if *yamlOut {
			yamlSong, err := yaml.Marshal(song)
			if err != nil {
				return fmt.Errorf("could not marshal the song as yaml file: %v", err)
			}
			if err := output(filename, ".yml", yamlSong); err != nil {
				return fmt.Errorf("error outputting yaml file: %v", err)
			}
		}
		return nil
	}
	retval := 0
	if *library {
		compiledLibrary, err := comp.Library()
		if err != nil {
			fmt.Fprintf(os.Stderr, "compiling library failed: %v\n", err)
			retval = 1
		} else {
			if len(*extensionsOut) > 0 {
				compiledLibrary = filterExtensions(compiledLibrary, strings.Split(*extensionsOut, ","))
			}
			for extension, code := range compiledLibrary {
				if err := output("sointu", extension, []byte(code)); err != nil {
					fmt.Fprintf(os.Stderr, "error outputting %v file: %v", extension, err)
					retval = 1
				}
			}
		}
	}
	for _, param := range flag.Args() {
		if info, err := os.Stat(param); err == nil && info.IsDir() {
			jsonfiles, err := filepath.Glob(filepath.Join(param, "*.json"))
			if err != nil {
				fmt.Fprintf(os.Stderr, "could not glob the path %v for json files: %v\n", param, err)
				retval = 1
				continue
			}
			ymlfiles, err := filepath.Glob(filepath.Join(param, "*.yml"))
			if err != nil {
				fmt.Fprintf(os.Stderr, "could not glob the path %v for yml files: %v\n", param, err)
				retval = 1
				continue
			}
			files := append(ymlfiles, jsonfiles...)
			for _, file := range files {
				err := process(file)
				if err != nil {
					fmt.Fprintf(os.Stderr, "could not process file %v: %v\n", file, err)
					retval = 1
				}
			}
		} else {
			err := process(param)
			if err != nil {
				fmt.Fprintf(os.Stderr, "could not process file %v: %v\n", param, err)
				retval = 1
			}
		}
	}
	os.Exit(retval)
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Sointu compiler. Input .yml or .json songs, outputs compiled songs (e.g. .asm and .h files).\nUsage: %s [flags] [path ...]\n", os.Args[0])
	flag.PrintDefaults()
}