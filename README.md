# slowtail 🐕

> A little tool to slow the tail :)

Have you ever experienced difficulties catching the tail?

![Fast tail](./img/fast_tail.gif)

**Struggle no more!**

## Installation

```sh
$ go get github.com/smileart/slowtail
```

## Usage
```sh
$ slowtail --help                                                                                             

Slow Tail 🐕

  Usage:
    slowtail [--delay=<ms>] [--rewind=<n>] <file>
    slowtail --help
    slowtail --version

  Options:
    --delay=<ms>, -d=<ms>  Delay in milliseconds [default: 250]
    --rewind=<n>, -r=<n>   Rewind <n> lines back from the end of file [default: 0]
                           Keep in mind: you can't rewind STDIN but you can skip <n>
                           lines from the beginning using this option
```

```sh
$ tail -f /var/log/fast.log | slowtail -d 2000 -
$ slowtail -d 2000 -r 1000 - < /var/log/long.log
$ slowtail -d 2000 /var/log/nginx/access.log
$ slowtail -d 2000 - < <(ls)
```

**You can even have a slow cat! SLOW CAT, Carl!**

```sh
$ cat /var/log/fast.log | slowtail -d 1000 -r 100 -
```

![Fast tail](./img/slow_cat.gif)


## License

MIT © [Serge Bedzhyk](http://github.com/smileart)
