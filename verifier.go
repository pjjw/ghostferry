package ghostferry

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type Verifier interface {
	StartVerification(*Ferry)
	VerificationStarted() bool
	VerificationDone() bool
	VerifiedCorrect() (bool, error)
	MismatchedTables() ([]string, error)
	Wait()
}

type ChecksumTableVerifier struct {
	*sync.WaitGroup

	StartTime time.Time
	DoneTime  time.Time

	TablesToCheck []string

	mismatchedTables []string
	err              error

	logger *logrus.Entry
}

func (this *ChecksumTableVerifier) StartVerification(f *Ferry) {
	this.WaitGroup = &sync.WaitGroup{}
	this.logger = logrus.WithField("tag", "checksum_verifier")
	this.Add(1)
	go func() {
		defer this.Done()
		this.Run(f)
	}()
}

func (this *ChecksumTableVerifier) Run(f *Ferry) {
	this.StartTime = time.Now()
	defer func() {
		this.DoneTime = time.Now()
	}()

	this.mismatchedTables = make([]string, 0)
	this.err = nil

	sourceTableChecksums := make(map[string]int64)
	query := fmt.Sprintf("CHECKSUM TABLE %s EXTENDED", strings.Join(this.TablesToCheck, ", "))

	sourceRows, err := f.SourceDB.Query(query)
	if err != nil {
		this.logger.WithError(err).Error("failed to checksum source tables")
		this.err = err
		return
	}

	defer sourceRows.Close()

	for sourceRows.Next() {
		var tablename string
		var checksum int64

		err = sourceRows.Scan(&tablename, &checksum)
		if err != nil {
			this.logger.WithError(err).Error("failed to scan row during source checksum tables")
			this.err = err
			return
		}

		sourceTableChecksums[tablename] = checksum
	}

	targetRows, err := f.TargetDB.Query(query)
	if err != nil {
		this.logger.WithError(err).Error("failed to checksum target tables")
		this.err = err
		return
	}
	defer targetRows.Close()

	for targetRows.Next() {
		var tablename string
		var checksum int64

		err = targetRows.Scan(&tablename, &checksum)
		if err != nil {
			this.logger.WithError(err).Error("failed to scan rows during target checksum tables")
			this.err = err
			return
		}

		this.logger.Debugf("source table checksum: %d | target table checksum: %d", sourceTableChecksums[tablename], checksum)
		if checksum != sourceTableChecksums[tablename] {
			this.logger.WithField("table", tablename).Warn("table verification failed")
			this.mismatchedTables = append(this.mismatchedTables, tablename)
		}
	}

}

func (this *ChecksumTableVerifier) VerificationStarted() bool {
	return !this.StartTime.IsZero()
}

func (this *ChecksumTableVerifier) VerificationDone() bool {
	return !this.DoneTime.IsZero()
}

func (this *ChecksumTableVerifier) VerifiedCorrect() (bool, error) {
	return len(this.mismatchedTables) == 0, this.err
}

func (this *ChecksumTableVerifier) MismatchedTables() ([]string, error) {
	return this.mismatchedTables, this.err
}