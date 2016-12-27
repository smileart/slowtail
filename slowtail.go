package main

import (
	"bufio"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/docopt/docopt-go"
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

func eachLine(filePath string, callback func(lineNum int, line string) error) (linesRead int, err error) {
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

func checkErr(err error) {
	if err != nil {
		log.Fatal("ERROR: ", err)
	}
}

func isStdinPath(filePath string) bool {
	stat, _ := os.Stdin.Stat()
	// fmt.Println(stat.Mode())
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		return true
	}

	return false
}

func sleepMilliseconds(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
	// fmt.Printf("I'll sleep for %vms\n", ms)
}

func main() {
	arguments, _ := docopt.Parse(doc, nil, true, version, false)
	fmt.Println(arguments)

	rewindLines := int(0)
	delayMilliseconds := int(250)

	if rewindArg, ok := arguments["--rewind"].(string); ok {
		rewindLines, _ = strconv.Atoi(rewindArg)
	}

	if delay, ok := arguments["--delay"].(string); ok {
		delayMilliseconds, _ = strconv.Atoi(delay)
	}

	if filePath, ok := arguments["<file>"].(string); ok {
		fmt.Println(isStdinPath(filePath))

		if isStdinPath(filePath) {
			readline.ReadLine(os.Stdin, func(line string) {
				fmt.Printf("%s\n", line)
				if rewindLines == 0 {
					sleepMilliseconds(delayMilliseconds)
				}

				if rewindLines > 0 {
					rewindLines--
				}
			})
		} else {
			if rewindLines > 0 {
				linesCount, err := eachLine(filePath, func(lineNum int, line string) error { return nil })
				linesCount = int(math.Abs(float64(rewindLines - linesCount)))

				if err == nil {
					eachLine(filePath, func(lineNum int, line string) error {
						if lineNum >= linesCount {
							fmt.Println(line)
						}

						return nil
					})
				} else {
					checkErr(err)
				}
			}

			t, err := tail.TailFile(filePath, tail.Config{
				Follow:   true,
				Poll:     true,
				Location: &tail.SeekInfo{Offset: 0, Whence: 2},
				Logger:   tail.DiscardingLogger,
			})

			if err == nil {
				for line := range t.Lines {
					fmt.Println(line.Text)
					sleepMilliseconds(delayMilliseconds)
				}
			} else {
				checkErr(err)
			}
		}
	}
}
