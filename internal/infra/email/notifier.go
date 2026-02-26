package email

import (
	"context"
	"fmt"
	"net/smtp"

	"go.uber.org/zap"
)

type SMTPNotifier struct {
	host   string
	port   int
	from   string
	logger *zap.Logger
}

func NewSMTPNotifier(host string, port int, from string, logger *zap.Logger) *SMTPNotifier {
	return &SMTPNotifier{host: host, port: port, from: from, logger: logger}
}

func (n *SMTPNotifier) NotifyFailure(_ context.Context, userEmail, jobID, videoKey, errorMsg string) error {
	addr := fmt.Sprintf("%s:%d", n.host, n.port)

	subject := fmt.Sprintf("FIAP X - Video Processing Failed [Job %s]", jobID)
	body := fmt.Sprintf(
		"Hello,\r\n\r\n"+
			"Your video processing job has permanently failed after all retry attempts.\r\n\r\n"+
			"Job ID: %s\r\n"+
			"Video: %s\r\n"+
			"Error: %s\r\n\r\n"+
			"Please try uploading the video again or contact support.\r\n\r\n"+
			"-- FIAP X Processing Service",
		jobID, videoKey, errorMsg,
	)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s",
		n.from, userEmail, subject, body,
	)

	err := smtp.SendMail(addr, nil, n.from, []string{userEmail}, []byte(msg))
	if err != nil {
		n.logger.Error("failed to send failure notification email",
			zap.String("to", userEmail),
			zap.String("job_id", jobID),
			zap.Error(err),
		)
		return fmt.Errorf("send email: %w", err)
	}

	n.logger.Info("failure notification email sent",
		zap.String("to", userEmail),
		zap.String("job_id", jobID),
	)
	return nil
}
