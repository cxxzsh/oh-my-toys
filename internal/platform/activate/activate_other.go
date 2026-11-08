//go:build !darwin

package activate

func frontmost() int { return 0 }
func activate(int)   {}
func activateSelf()  {}
