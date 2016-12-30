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
	"sync"
	"time"

	docopt "github.com/docopt/docopt-go"
	color "github.com/fatih/color"
	tail "github.com/hpcloud/tail"
	readline "github.com/jprichardson/readline-go"
	termbox "github.com/nsf/termbox-go"
)

const version = "Slow Tail v0.1"

const doc = `Slow Tail 🐕

  Usage:
    slowtail [--delay=<ms>] [--rewind=<n>] [--interactive] [--porcelain] <file>
    slowtail --help
    slowtail --version

  Options:
    --interactive, -i      Interactive mode ( ⬆⬇ to make the flow faster/slower )
    --porcelain, -p        Human friendly output in interactive mode 🚽
                           Beware: output shouldn't be used with other commands!
    --delay=<ms>, -d=<ms>  Delay in milliseconds [default: 250]
    --rewind=<n>, -r=<n>   Rewind <n> lines back from the end of file [default: 0]
                           Keep in mind: you can't rewind STDIN but you can skip <n>
                           lines from the beginning using this option`

type arguments struct {
	rewindLines       int
	delayMilliseconds int
	filePath          string
}

var globalDelay = 0
var globalDelayMutex = &sync.Mutex{}

func main() {
	linesChannel := make(chan string, 1)
	readyChannel := make(chan bool, 1)

	args, _ := docopt.Parse(doc, nil, true, version, false)
	options, err := parseArgs(args)

	if err != nil {
		checkErr(err)
	}

	globalDelay = options.delayMilliseconds

	if len(options.filePath) > 0 {
		if args["--interactive"] == true {
			go interactiveMode(&readyChannel, args["--porcelain"] == true)
		} else {
			readyChannel <- true
		}

		if isStdin() && <-readyChannel {
			go stdinToChan(os.Stdin, &linesChannel, options.rewindLines)
		} else {
			go fileToChan(options.filePath, &linesChannel, options.rewindLines)
		}

		for line := range linesChannel {
			fmt.Println(line)
		}
	}
}

func parseArgs(args map[string]interface{}) (arguments, error) {

	rewindLines := int(0)
	delayMilliseconds := int(250)
	filePath := ""

	if rewindArg, ok := args["--rewind"].(string); ok {
		rewindLines, _ = strconv.Atoi(rewindArg)
	}

	if rewindLines < 0 || rewindLines > math.MaxInt32 {
		return arguments{}, errors.New("--rewind must be a positive number of lines")
	}

	if delayArg, ok := args["--delay"].(string); ok {
		delayMilliseconds, _ = strconv.Atoi(delayArg)
	}

	if delayMilliseconds < 0 || delayMilliseconds > math.MaxInt32 {
		return arguments{}, errors.New("--delay must be a positive number of milliseconds")
	}

	if filePathArg, ok := args["<file>"].(string); ok {
		filePath = filePathArg
	}

	return arguments{
		rewindLines,
		delayMilliseconds,
		filePath,
	}, nil
}

func stdinToChan(source io.Reader, linesChannel *chan string, rewindLinesCount int) {
	defer close(*linesChannel)

	readline.ReadLine(os.Stdin, func(line string) {
		*linesChannel <- line

		if rewindLinesCount == 0 {
			sleepMilliseconds(globalDelay)
		}

		if rewindLinesCount > 0 {
			rewindLinesCount--
		}
	})
}

func fileToChan(source string, linesChannel *chan string, rewindLinesCount int) {
	defer close(*linesChannel)

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
		checkErr(err)
	}
}

func eachFileLine(filePath string, callback func(lineNum int, line string) error) (linesRead int, err error) {
	file, err := os.Open(filePath)
	defer file.Close()

	if err != nil {
		return 0, err
	}

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

func interactiveMode(readyChannel *chan (bool), humanFriendly bool) {
	err := termbox.Init()
	if err != nil {
		checkErr(err)
	}

	termbox.SetInputMode(termbox.InputCurrent)
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)

	*readyChannel <- true

	for {
		switch ev := termbox.PollEvent(); ev.Type {
		case termbox.EventKey:
			if ev.Key == termbox.KeyArrowDown {
				changeSpeed(true, humanFriendly)
			}

			if ev.Key == termbox.KeyArrowUp {
				changeSpeed(false, humanFriendly)
			}

			if ev.Key == termbox.KeyCtrlC {
				termbox.Flush()
				termbox.Close()
				os.Exit(0)
			}
		case termbox.EventError:
			checkErr(ev.Err)
		}
	}
}

func speedMessage(down bool) string {
	direction := "faster"
	if down == true {
		direction = "slower"
	}

	globalDelayMutex.Lock()
	defer globalDelayMutex.Unlock()

	w, _ := termbox.Size()
	var format string
	realTimeMsg := "Working in real-time…"

	switch {
	case w >= 70:
		format = "━━━━━━━━━━━━━━━━ Going %v (delay: %[2]v ms) ━━━━━━━━━━━━━━━━"
		realTimeMsg = "━━━━━━━━━━━━━━━━━━━ " + realTimeMsg + " ━━━━━━━━━━━━━━━━━━━"
	case w < 70 && w >= 55:
		format = "━━━━━━━━━ Going %v (delay: %[2]v ms) ━━━━━━━━━"
		realTimeMsg = "━━━━━━━━━━━ " + realTimeMsg + " ━━━━━━━━━━━"
	case w < 55 && w >= 40:
		format = "━━ Going %v (delay: %[2]v ms) ━━"
		realTimeMsg = "━━ " + realTimeMsg + " ━━"
	case w < 40 && w >= 20:
		format = "━ %[2]v ms ━"
		realTimeMsg = "━━━ RT ━━━"
	default:
		return ""
	}

	if globalDelay > 0 {
		return fmt.Sprintf(format, direction, globalDelay)
	}

	return realTimeMsg
}

func changeSpeed(down bool, humanFriendly bool) {
	if down {
		if globalDelay < math.MaxInt32-250 {
			globalDelayMutex.Lock()
			globalDelay += 250
			globalDelayMutex.Unlock()
		}
	} else {
		globalDelayMutex.Lock()
		if globalDelay-250 >= 0 {
			globalDelay -= 250
		}
		globalDelayMutex.Unlock()
	}

	if humanFriendly {
		fmt.Println(speedMessage(down))
	}
}

func isStdin() bool {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		return true
	}

	return false
}

func checkErr(err error) {
	if err != nil {
		log.Fatal(color.RedString("ERROR: "), color.RedString(err.Error()))
	}
}

func sleepMilliseconds(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}
