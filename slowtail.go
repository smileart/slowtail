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
	porcelain         bool
	interactive       bool
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

	if options.interactive == true {
		go interactiveMode(&readyChannel, options.porcelain == true)
	} else {
		readyChannel <- true
	}

	if <-readyChannel {
		if isStdin() {
			go stdinToChan(os.Stdin, &linesChannel, options.rewindLines)
		} else {
			go fileToChan(options.filePath, &linesChannel, options.rewindLines)
		}
	}

	for line := range linesChannel {
		fmt.Println(line)
	}
}

func parseArgs(args map[string]interface{}) (arguments, error) {
	rewindLines := int(0)
	delayMilliseconds := int(250)
	filePath := ""
	porcelain := false
	interactive := false

	if rewindArg, ok := args["--rewind"].(string); ok {
		rewindLines, _ = strconv.Atoi(rewindArg)
	}

	if porcelainArg, ok := args["--porcelain"].(bool); ok {
		porcelain = porcelainArg
	}

	if interactiveArg, ok := args["--interactive"].(bool); ok {
		interactive = interactiveArg
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
		porcelain,
		interactive,
	}, nil
}

func stdinToChan(source io.Reader, linesChannel *chan string, rewindLinesCount int) {
	scanner := bufio.NewScanner(source)

	for scanner.Scan() {
		*linesChannel <- scanner.Text()

		if rewindLinesCount <= 0 {
			sleepMilliseconds(globalDelay)
		}

		if rewindLinesCount > 0 {
			rewindLinesCount--
		}
	}
}

func fileToChan(source string, linesChannel *chan string, rewindLinesCount int) {
	defer close(*linesChannel)

	if _, err := os.Stat(source); os.IsNotExist(err) {
		checkErr(err)
	}

	if rewindLinesCount > 0 {
		tailFile(source, rewindLinesCount)
	}

	t, err := tail.TailFile(source, tail.Config{
		Follow:   true,
		Poll:     true,
		Location: &tail.SeekInfo{Offset: 0, Whence: 2},
		Logger:   tail.DiscardingLogger,
	})

	if err != nil {
		checkErr(err)
	}

	for line := range t.Lines {
		*linesChannel <- line.Text
		sleepMilliseconds(globalDelay)
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
				quitInteracitve(humanFriendly)
				break
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
		format = "━━━━━━━━━━━━━━━━ Going %[1]v (delay: %[2]v ms) ━━━━━━━━━━━━━━━━"
		realTimeMsg = "━━━━━━━━━━━━━━━━━━━ " + realTimeMsg + " ━━━━━━━━━━━━━━━━━━━"
	case w < 70 && w >= 55:
		format = "━━━━━━━━━ Going %[1]v (delay: %[2]v ms) ━━━━━━━━━"
		realTimeMsg = "━━━━━━━━━━━ " + realTimeMsg + " ━━━━━━━━━━━"
	case w < 55 && w >= 40:
		format = "━━ Going %[1]v (delay: %[2]v ms) ━━"
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

func quitInteracitve(humanFriendly bool) {
	termbox.Flush()
	termbox.Close()

	if humanFriendly {
		fmt.Println("Press Ctrl+C to exit…")
	}

	os.Stdout.Close()
	os.Stdin.Close()
}

func isStdin() bool {
	stat, _ := os.Stdin.Stat()

	if (stat.Mode() & os.ModeCharDevice) == os.ModeCharDevice {
		return false
	}

	return true
}

func checkErr(err error) {
	if err != nil {
		log.Fatal(color.RedString("ERROR: "), color.RedString(err.Error()))
	}
}

func sleepMilliseconds(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}
