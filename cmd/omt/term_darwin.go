package main

// ioctlReadTermios is TIOCGETA: read the terminal settings. It fails on a
// character device that is not a terminal, such as /dev/null.
const ioctlReadTermios = 0x40487413
