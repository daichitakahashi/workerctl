package workerctl

import "log"

// Go :
func Go(f func()) {
	go func() {
		defer func() {
			rvr := recover()
			if rvr != nil {
				// TODO: グローバルロガー
				log.Println("workerctl: go:", rvr)
			}
		}()
		f()
	}()
}

// Closer :
type Closer []func() error

// Append :
func (c *Closer) Append(f func() error) {
	*c = append(*c, f)
}

// Close :
func (c *Closer) Close() (err error) {
	for _, closeFn := range *c {
		cerr := closeFn()
		if cerr != nil && err == nil {
			err = cerr
		}
	}
	return
}
