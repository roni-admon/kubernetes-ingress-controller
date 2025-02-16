package sendconfig

import (
	"context"
	"fmt"
	"time"

	"github.com/kong/deck/file"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kong/kubernetes-ingress-controller/v2/internal/adminapi"
	"github.com/kong/kubernetes-ingress-controller/v2/internal/dataplane/deckgen"
	"github.com/kong/kubernetes-ingress-controller/v2/internal/dataplane/failures"
	"github.com/kong/kubernetes-ingress-controller/v2/internal/metrics"
)

// -----------------------------------------------------------------------------
// Sendconfig - Public Functions
// -----------------------------------------------------------------------------

type UpdateStrategyResolver interface {
	ResolveUpdateStrategy(client UpdateClient) UpdateStrategy
}

// PerformUpdate writes `targetContent` to Kong Admin API specified by `kongConfig`.
func PerformUpdate(
	ctx context.Context,
	log logrus.FieldLogger,
	client *adminapi.Client,
	config Config,
	targetContent *file.Content,
	promMetrics *metrics.CtrlFuncMetrics,
	updateStrategyResolver UpdateStrategyResolver,
	configChangeDetector ConfigurationChangeDetector,
) ([]byte, []failures.ResourceFailure, error) {
	oldSHA := client.LastConfigSHA()
	newSHA, err := deckgen.GenerateSHA(targetContent)
	if err != nil {
		return oldSHA, []failures.ResourceFailure{}, err
	}

	// disable optimization if reverse sync is enabled
	if !config.EnableReverseSync {
		configurationChanged, err := configChangeDetector.HasConfigurationChanged(ctx, oldSHA, newSHA, client, client.AdminAPIClient())
		if err != nil {
			return nil, []failures.ResourceFailure{}, err
		}
		if !configurationChanged {
			log.Debug("no configuration change, skipping sync to Kong")
			return oldSHA, []failures.ResourceFailure{}, nil
		}
	}

	updateStrategy := updateStrategyResolver.ResolveUpdateStrategy(client)
	timeStart := time.Now()
	err, resourceErrors, resourceErrorsParseErr := updateStrategy.Update(ctx, targetContent)
	duration := time.Since(timeStart)

	metricsProtocol := updateStrategy.MetricsProtocol()
	if err != nil {
		resourceFailures := resourceErrorsToResourceFailures(resourceErrors, resourceErrorsParseErr, log)
		promMetrics.RecordPushFailure(metricsProtocol, duration, client.BaseRootURL(), err)
		return nil, resourceFailures, err
	}

	promMetrics.RecordPushSuccess(metricsProtocol, duration, client.BaseRootURL())
	log.Info("successfully synced configuration to kong")
	return newSHA, nil, nil
}

// -----------------------------------------------------------------------------
// Sendconfig - Private Functions
// -----------------------------------------------------------------------------

// resourceErrorsToResourceFailures translates a slice of ResourceError to a slice of failures.ResourceFailure.
// In case of parseErr being not nil, it just returns a nil slice.
func resourceErrorsToResourceFailures(resourceErrors []ResourceError, parseErr error, log logrus.FieldLogger) []failures.ResourceFailure {
	if parseErr != nil {
		log.WithError(parseErr).Error("failed parsing resource errors")
		return nil
	}

	var out []failures.ResourceFailure
	for _, ee := range resourceErrors {
		obj := metav1.PartialObjectMetadata{
			TypeMeta: metav1.TypeMeta{
				Kind:       ee.Kind,
				APIVersion: ee.APIVersion,
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ee.Namespace,
				Name:      ee.Name,
				UID:       types.UID(ee.UID),
			},
		}
		for field, problem := range ee.Problems {
			log.Debug(fmt.Sprintf("adding failure for %s: %s = %s", ee.Name, field, problem))
			resourceFailure, failureCreateErr := failures.NewResourceFailure(
				fmt.Sprintf("invalid %s: %s", field, problem),
				&obj,
			)
			if failureCreateErr != nil {
				log.WithError(failureCreateErr).Error("could create resource failure event")
			} else {
				out = append(out, resourceFailure)
			}
		}
	}

	return out
}
