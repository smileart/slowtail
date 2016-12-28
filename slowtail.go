package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/docopt/docopt-go"
	"github.com/fatih/color"
	"github.com/hpcloud/tail"
	"github.com/jprichardson/readline-go"
)

const version = "Slow Tail v1.0"

const doc = `Slow Tail 🐕

  Usage:
    slowtail [--delay=<ms>] [--rewind=<n>] <file>
    slowtail --help
    slowtail --version

  Options:
    --delay=<ms>, -d=<ms>  Delay in milliseconds [default: 250]
    --rewind=<n>, -r=<n>   Rewind <n> lines back from the end of file [default: 0]
                           Keep in mind: you can't rewind STDIN but you can skip <n>
                           lines from the beginning using this option`

type options struct {
	rewindLines       int
	delayMilliseconds int
	filePath          string
}

var globalDelay = 0

func eachFileLine(filePath string, callback func(lineNum int, line string) error) (linesRead int, err error) {
	file, err := os.Open(filePath)

	if err != nil {
		return 0, err
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		err := callback(lineNum, scanner.Text())

		if err != nil {
			return lineNum, err
		}

		lineNum++
	}

	if err := scanner.Err(); err != nil {
		return lineNum, err
	}

	return lineNum, nil
}

func tailFile(filePath string, linesCount int) {
	totalLinesCount, err := eachFileLine(filePath, func(lineNum int, line string) error { return nil })
	linesToTail := int(math.Abs(float64(linesCount - totalLinesCount)))

	if err == nil {
		eachFileLine(filePath, func(lineNum int, line string) error {
			if lineNum >= linesToTail {
				fmt.Println(line)
			}

			return nil
		})
	} else {
		checkErr(err)
	}
}

func checkErr(err error) {
	if err != nil {
		log.Fatal(color.RedString("ERROR: "), color.RedString(err.Error()))
	}
}

func isStdinPath(filePath string) bool {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		return true
	}

	return false
}

func sleepMilliseconds(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

func parseArgs(args map[string]interface{}) (options, error) {

	rewindLines := int(0)
	delayMilliseconds := int(250)
	filePath := ""

	if rewindArg, ok := args["--rewind"].(string); ok {
		rewindLines, _ = strconv.Atoi(rewindArg)
	}

	if rewindLines < 0 || rewindLines > math.MaxInt32 {
		return options{}, errors.New("--rewind must be a positive number of lines")
	}

	if delayArg, ok := args["--delay"].(string); ok {
		delayMilliseconds, _ = strconv.Atoi(delayArg)
	}

	if delayMilliseconds < 0 || delayMilliseconds > math.MaxInt32 {
		return options{}, errors.New("--delay must be a positive number of milliseconds")
	}

	if filePathArg, ok := args["<file>"].(string); ok {
		filePath = filePathArg
	}

	return options{
		rewindLines,
		delayMilliseconds,
		filePath,
	}, nil
}

func stdinToChan(source io.Reader, linesChannel *chan string, rewindLinesCount int) {
	readline.ReadLine(os.Stdin, func(line string) {
		*linesChannel <- line

		if rewindLinesCount == 0 {
			sleepMilliseconds(globalDelay)
		}

		if rewindLinesCount > 0 {
			rewindLinesCount--
		}
	})

	close(*linesChannel)
}

func fileToChan(source string, linesChannel *chan string, rewindLinesCount int) {
	if rewindLinesCount > 0 {
		tailFile(source, rewindLinesCount)
	}

	t, err := tail.TailFile(source, tail.Config{
		Follow:   true,
		Poll:     true,
		Location: &tail.SeekInfo{Offset: 0, Whence: 2},
		Logger:   tail.DiscardingLogger,
	})

	if err == nil {
		for line := range t.Lines {
			*linesChannel <- line.Text
			sleepMilliseconds(globalDelay)
		}
	} else {
		close(*linesChannel)
		checkErr(err)
	}
}

func main() {
	arguments, _ := docopt.Parse(doc, nil, true, version, false)

	options, err := parseArgs(arguments)

	if err != nil {
		checkErr(err)
	}

	globalDelay = options.delayMilliseconds

	if len(options.filePath) > 0 {
		linesChannel := make(chan string, 1)

		if isStdinPath(options.filePath) {
			go stdinToChan(os.Stdin, &linesChannel, options.rewindLines)
		} else {
			go fileToChan(options.filePath, &linesChannel, options.rewindLines)
		}

		for line := range linesChannel {
			fmt.Println(line)
		}
	}
}
