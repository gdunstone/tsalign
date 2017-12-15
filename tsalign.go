package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/rwcarlsen/goexif/exif"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	tsForm         = "2006_01_02_15_04_05"
	dumbExifForm   = "2006:01:02 15:04:05"
	tsDirStruct    = "2006/2006_01/2006_01_02/2006_01_02_15/"
	tsRegexPattern = "[0-9][0-9][0-9][0-9]_[0-1][0-9]_[0-3][0-9]_[0-2][0-9]_[0-5][0-9]_[0-5][0-9]"
)

var (
	interval           time.Duration
	rootDir, outputDir string
	del, stdin         bool
	datetimeFunc       datetimeFunction
)

var /* const */ tsRegex = regexp.MustCompile(tsRegexPattern)

func ERRLOG(format string, a ...interface{}) (n int, err error) {
	return fmt.Fprintf(os.Stderr, format+"\n", a...)
}

func OUTPUT(a ...interface{}) (n int, err error) {
	return fmt.Fprintln(os.Stdout, a...)
}

func moveFilebyCopy(src, dst string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	// no need to check errors on read only file, we already got everything
	// we need from the filesystem, so nothing can go wrong now.
	defer s.Close()

	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(d, s); err != nil {
		d.Close()
		return err
	}
	if del {
		absSrc, _ := filepath.Abs(src)
		absDest, _ := filepath.Abs(dst)
		if absSrc != absDest {
			os.Remove(src)
		}
	}
	return d.Close()
}

type datetimeFunction func(string) (time.Time, error)

func parseExifDatetime(datetimeString string) (time.Time, error) {
	thisTime, err := time.Parse(dumbExifForm, datetimeString)
	if err != nil {
		return time.Time{}, err
	}
	return thisTime, nil
}

type ExifFromJSON struct {
	DateTime          string
	DateTimeOriginal  string
	DateTimeDigitized string
}

func getTimeFromExif(thisFile string) (datetime time.Time, err error) {

	var datetimeString string
	if _, ferr := os.Stat(thisFile + ".json"); ferr == nil {
		eData := ExifFromJSON{}
		//	do something with the json.

		byt, err := ioutil.ReadFile(thisFile + ".json")
		if err != nil {
			ERRLOG("[json] cant read file %s", err)
		}
		if err := json.Unmarshal(byt, &eData); err != nil {
			ERRLOG("[json] can't unmarshal %s", err)
		}

		datetimeString = eData.DateTime

	} else {
		fileHandler, err := os.Open(thisFile)
		if err != nil {

			// file wouldnt open
			return time.Time{}, err
		}
		exifData, err := exif.Decode(fileHandler)
		if err != nil {
			// exif wouldnt decode
			return time.Time{}, errors.New(fmt.Sprintf("[exif] couldn't decode exif from image %s", err))
		}
		dt, err := exifData.Get(exif.DateTime) // normally, don't ignore errors!
		if err != nil {
			// couldnt get DateTime from exifex
			return time.Time{}, err
		}
		datetimeString, err = dt.StringVal()
		if err != nil {
			// couldnt get
			return time.Time{}, err
		}
	}
	if datetime, err = parseExifDatetime(datetimeString); err != nil {
		ERRLOG("[parse] parse datetime %s", err)
	}
	return
}

func getTimeFromFileTimestamp(thisFile string) (time.Time, error) {
	timestamp := tsRegex.FindString(thisFile)
	if len(timestamp) < 1 {
		// no timestamp found in filename
		return time.Time{}, errors.New("failed regex timestamp from filename")
	}

	t, err := time.Parse(tsForm, timestamp)
	if err != nil {
		// parse error
		return time.Time{}, err
	}
	return t, nil
}

func alignedFilename(thisFile string) (string, error) {
	thisTime, err := datetimeFunc(thisFile)
	if err != nil {
		return "", err
	}

	aligned := thisTime.Truncate(interval)

	if err != nil {
		return "", err
	}

	targetFilename := strings.Replace(thisFile, thisTime.Format(tsForm), aligned.Format(tsForm), 1)
	// make sure that if its already formatted as a timestream that we reformat the timestream structure.
	targetFilename = strings.Replace(targetFilename, thisTime.Format(tsDirStruct), aligned.Format(tsDirStruct), 1)

	return path.Join(outputDir, targetFilename), nil
}

func moveOrRename(source, dest string) error {
	// rename/copy+del if del is true otherwise moveFilebyCopy to not del.
	var err error
	if del {
		err = os.Rename(source, dest)
		if err != nil {
			err = moveFilebyCopy(source, dest)
		}
	} else {
		err = moveFilebyCopy(source, dest)
	}
	if err != nil {
		ERRLOG("[move] %s", err)
		return nil
	}
	return err
}

func visit(filePath string, info os.FileInfo, _ error) error {
	// skip directories
	if info.IsDir() {
		return nil
	}
	if path.Ext(filePath) == ".json" {
		return nil
	}

	// parse the new filepath
	newPath, err := alignedFilename(filePath)
	if err != nil {
		ERRLOG("[parse] %s", err)
		return nil
	}

	if _, err := os.Stat(newPath); err == nil {
		// skip existing.
		return nil
	}

	// make directories
	err = os.MkdirAll(path.Dir(newPath), 0755)
	if err != nil {
		ERRLOG("[mkdir] %s", err)
		return nil
	}

	absSrc, _ := filepath.Abs(filePath)
	absDest, _ := filepath.Abs(newPath)
	if absSrc == absDest {
		ERRLOG("[dupe] %s", absDest)
		return nil
	}

	err = moveOrRename(filePath, absDest)
	jsFile := filePath + ".json"
	if _, ferr := os.Stat(jsFile); ferr == nil {
		if e := moveOrRename(jsFile, absDest+".json"); e != nil {
			ERRLOG("[exif] couldn't move json exif file")
		}
	}

	OUTPUT(newPath)

	return err
}

var usage = func() {
	ERRLOG("usage of %s:", os.Args[0])
	ERRLOG("\talign images in place:")
	ERRLOG("\t\t -source <source> -output <source>", os.Args[0])
	ERRLOG("\t copy aligned to <destination>:")
	ERRLOG("\t\t %s -source <source> -output=<destination>", os.Args[0])

	ERRLOG("")
	ERRLOG("flags:")
	ERRLOG("\t-name: renames the prefix fo the target files")
	ERRLOG("\t-exif: uses exif data to rename rather than file timestamp")
	pwd, _ := os.Getwd()
	ERRLOG("\t-output: set the <destination> directory (default=%s)", pwd)
	ERRLOG("\t-source: set the <source> directory (optional, default=stdin)", pwd)
	ERRLOG("\t-interval: set the interval to align to (optional, default=5m)", pwd)
	ERRLOG("")
	ERRLOG("reads filepaths from stdin")
	ERRLOG("will ignore any line from stdin that isnt a filepath (and only a filepath)")

	ERRLOG("")
	ERRLOG("will only align down, if an image is at 10:03 (5m interval) it will align to 10:00")
	ERRLOG("chronologically earlier images will be kept")
	ERRLOG("ie. at 5m interval, an image at 10:03 will overwrite an image at 10:02")

}

func init() {
	flag.Usage = usage
	// set flags for flagset

	flag.DurationVar(&interval, "interval", time.Minute*5, "interval to align to.")
	flag.StringVar(&rootDir, "source", "", "source directory")
	flag.StringVar(&outputDir, "output", ".", "output directory")

	useExif := flag.Bool("exif", false, "use exif instead of timestamps in filenames")
	// parse the leading argument with normal flag.Parse
	flag.Parse()

	if *useExif {
		datetimeFunc = getTimeFromExif
	} else {
		datetimeFunc = getTimeFromFileTimestamp
	}

	if rootDir != "" {
		if _, err := os.Stat(rootDir); err != nil {
			if os.IsNotExist(err) {
				ERRLOG("[path] <source> %s does not exist.", rootDir)
				os.Exit(1)
			}
		}
	}

	os.MkdirAll(outputDir, 0755)

	stdin = rootDir == ""

	outputAbs, _ := filepath.Abs(outputDir)
	absRoot, _ := filepath.Abs(rootDir)

	// if output and source are the same then it is an in place rename.
	del = absRoot == outputAbs

}

func main() {
	if !stdin {
		if err := filepath.Walk(rootDir, visit); err != nil {
			ERRLOG("[walk] %s", err)
		}
	} else {
		// start scanner and wait for stdin
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			text := strings.Replace(scanner.Text(), "\n", "", -1)
			if strings.HasPrefix(text, "[") {
				ERRLOG("[stdin] %s", text)
				continue
			} else {
				finfo, err := os.Stat(text)
				if err != nil {
					ERRLOG("[stat] %s", text)
					continue
				}
				visit(text, finfo, nil)
			}
		}
	}

	//c := make(chan error)
	//go func() {
	//	c <- filepath.Walk(rootDir, visit)
	//}()
	//
	//if err := <-c; err != nil {
	//	fmt.Println(err)
	//}
}
