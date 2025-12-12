package logger

import "go.uber.org/zap"

func NewLogger() (*zap.Logger, error) {
	logger, err := zap.NewProduction(zap.AddCaller(), zap.AddCallerSkip(1))
	if err != nil {
		return nil, err
	}
	return logger, nil
}
