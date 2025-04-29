package healths

import "errors"

type Down struct{}

func (d Down) Name() string {
	return "down"
}

func (d Down) CheckLiveness() error {
	return nil
}

func (d Down) CheckReadiness() error {
	return errors.New("down")
}
