package mlerrors

import "errors"

var ErrNotBootstrapped = errors.New("ml detector not yet bootstrapped")
var ErrNoBaseline = errors.New("no fitted baseline for this metric")
