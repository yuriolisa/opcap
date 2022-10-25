package capability

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/opdev/opcap/internal/logger"
	"github.com/opdev/opcap/internal/operator"
)

func operatorInstall(ctx context.Context, opts ...auditOption) (auditFn, auditCleanupFn) {
	var options options
	for _, opt := range opts {
		err := opt(&options)
		if err != nil {
			return func(_ context.Context) error {
					return fmt.Errorf("option failed: %v", err)
				},
				func(_ context.Context) error {
					return nil
				}
		}
	}

	return func(ctx context.Context) error {
		logger.Debugw("installing package", "package", options.Subscription.Package, "channel", options.Subscription.Channel, "installmode", options.Subscription.InstallModeType)

		// create operator's own namespace
		if _, err := options.client.CreateNamespace(ctx, options.namespace); err != nil {
			return err
		}

		// create remaining target namespaces watched by the operator
		for _, ns := range options.operatorGroupData.TargetNamespaces {
			if ns != options.namespace {
				options.client.CreateNamespace(ctx, ns)
			}
		}

		// create operator group for operator package/channel
		options.client.CreateOperatorGroup(ctx, *options.operatorGroupData, options.namespace)

		// create subscription for operator package/channel
		if _, err := options.client.CreateSubscription(ctx, *options.Subscription, options.namespace); err != nil {
			logger.Debugf("Error creating subscriptions: %w", err)
			return err
		}

		// Get a Succeeded or Failed CSV with one minute timeout
		resultCSV, err := options.client.GetCompletedCsvWithTimeout(ctx, options.namespace, options.csvWaitTime)
		if err != nil {
			// If error is timeout than don't log phase but timeout
			if errors.Is(err, operator.TimeoutError) {
				options.CsvTimeout = true
			} else {
				return err
			}
		}
		options.Csv = resultCSV

		file, err := options.fs.OpenFile("operator_install_report.json", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer file.Close()

		if err := operatorInstallJsonReport(file, options); err != nil {
			return fmt.Errorf("could not generate operator install JSON report: %v", err)
		}

		if err := operatorInstallTextReport(os.Stdout, options); err != nil {
			return fmt.Errorf("could not generate operator install text report: %v", err)
		}

		return nil
	}, operatorCleanup(ctx, opts...)
}
