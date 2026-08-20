package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	history_api "github.com/runtime-radar/runtime-radar/history-api/api"
	"github.com/runtime-radar/runtime-radar/lib/errcommon"
	"github.com/runtime-radar/runtime-radar/lib/security/cipher"
	"github.com/runtime-radar/runtime-radar/notifier/api"
	aiclient "github.com/runtime-radar/runtime-radar/notifier/pkg/ai"
	"github.com/runtime-radar/runtime-radar/notifier/pkg/database"
	"github.com/runtime-radar/runtime-radar/notifier/pkg/model"
	"github.com/runtime-radar/runtime-radar/notifier/pkg/model/convert"
	"github.com/runtime-radar/runtime-radar/notifier/pkg/notifier"
	enforcer_api "github.com/runtime-radar/runtime-radar/policy-enforcer/api"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/emptypb"
	"gorm.io/gorm"
)

type IntegrationGeneric struct {
	api.UnimplementedIntegrationControllerServer

	IntegrationRepository  database.IntegrationRepository
	NotificationRepository database.NotificationRepository
	RuleController         enforcer_api.RuleControllerClient
	RuntimeHistory         history_api.RuntimeHistoryClient
	Crypter                cipher.Crypter
}

func (ig *IntegrationGeneric) Create(ctx context.Context, req *api.Integration) (*api.CreateIntegrationResp, error) {
	if reason, ok := validateIntegration(req); !ok {
		return nil, status.Error(codes.InvalidArgument, reason)
	}

	var (
		i   model.Integration
		err error
	)
	i, err = convert.IntegrationFromPB(req)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "can't parse integration: %v", err)
	}

	if !req.GetSkipCheck() {
		if err := ig.testIntegration(ctx, i); err != nil {
			msg := fmt.Sprintf("integration is inaccessible: %v", err)
			return nil, errcommon.StatusWithReason(codes.InvalidArgument, IntegrationInaccessible, msg).Err()
		}
	}

	// encrypt sensitive data before persisting to storage
	i.EncryptSensitive(ig.Crypter)

	if err := ig.IntegrationRepository.Add(ctx, i); err != nil {
		if errors.Is(err, model.ErrIntegrationNameInUse) {
			return nil, errcommon.StatusWithReason(codes.AlreadyExists, NameMustBeUnique, "name must be unique").Err()
		}

		return nil, status.Errorf(codes.Internal, "can't save integration: %v", err)
	}

	return &api.CreateIntegrationResp{Id: i.GetID().String()}, nil
}

func (ig *IntegrationGeneric) Read(ctx context.Context, req *api.ReadIntegrationReq) (*api.Integration, error) {
	if reason, ok := validateIntegrationType(req.GetType()); !ok {
		return nil, status.Error(codes.InvalidArgument, reason)
	}

	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "ID is not set")
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "can't parse ID: %v", err)
	}

	i, err := ig.IntegrationRepository.GetByTypeAndID(ctx, req.GetType(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "integration not found")
		}
		return nil, status.Errorf(codes.Internal, "can't get integration: %v", err)
	}

	return convert.IntegrationToPB(i, true), nil
}

func (ig *IntegrationGeneric) Update(ctx context.Context, req *api.Integration) (*emptypb.Empty, error) {
	if reason, ok := validateIntegration(req); !ok {
		return nil, status.Error(codes.InvalidArgument, reason)
	}

	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "ID is not set")
	}

	i, err := convert.IntegrationFromPB(req)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "can't parse integration: %v", err)
	}

	if !req.GetSkipCheck() {
		if err := ig.restoreStoredAPIKey(ctx, i); err != nil {
			return nil, status.Errorf(codes.Internal, "can't read stored integration: %v", err)
		}

		if err := ig.testIntegration(ctx, i); err != nil {
			msg := fmt.Sprintf("integration is inaccessible: %v", err)
			return nil, errcommon.StatusWithReason(codes.InvalidArgument, IntegrationInaccessible, msg).Err()
		}
	}

	var updateMap map[string]any

	switch conf := req.GetConfig().(type) {
	case *api.Integration_Email:
		updateMap = ig.emailUpdateMap(req.GetName(), conf.Email)
	case *api.Integration_Webhook:
		updateMap = ig.webhookUpdateMap(req.GetName(), conf.Webhook)
	case *api.Integration_Syslog:
		updateMap = ig.syslogUpdateMap(req.GetName(), conf.Syslog)
	case *api.Integration_Ai:
		updateMap = ig.aiUpdateMap(req.GetName(), conf.Ai)
	default:
		return nil, status.Errorf(codes.InvalidArgument, "invalid config type given: %T", conf)
	}

	if err := ig.IntegrationRepository.UpdateWithMap(ctx, req.GetType(), i.GetID(), updateMap); err != nil {
		if errors.Is(err, model.ErrIntegrationNameInUse) {
			return nil, errcommon.StatusWithReason(codes.AlreadyExists, NameMustBeUnique, "name must be unique").Err()
		}

		return nil, status.Errorf(codes.Internal, "can't update integration: %v", err)
	}

	return &emptypb.Empty{}, nil
}

func (ig *IntegrationGeneric) Delete(ctx context.Context, req *api.DeleteIntegrationReq) (*emptypb.Empty, error) {
	if reason, ok := validateIntegrationType(req.GetType()); !ok {
		return nil, status.Error(codes.InvalidArgument, reason)
	}

	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "ID is not set")
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "can't parse ID: %v", err)
	}

	notifications, err := ig.NotificationRepository.GetByIntegrationID(ctx, id, "")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "can't get integration's notifications: %v", err)
	}

	if len(notifications) > 0 {
		rcResp, err := ig.RuleController.NotifyTargetsInUse(ctx, &enforcer_api.NotifyTargetsInUseReq{
			Targets: idsFromNotifications(notifications...),
		})
		if err != nil {
			return nil, status.Errorf(codes.Internal, "can't check if notifications are in use: %v", err)
		}

		if rcResp.InUse {
			return nil, errcommon.StatusWithReason(codes.FailedPrecondition, NotificationInUse, "integration's notifications are in use by rule").Err()
		}
	}

	if err := ig.IntegrationRepository.DeleteByTypeAndID(ctx, req.GetType(), id); err != nil {
		return nil, status.Errorf(codes.Internal, "can't delete integration: %v", err)
	}

	return &emptypb.Empty{}, nil
}

func (ig *IntegrationGeneric) List(ctx context.Context, req *api.ListIntegrationReq) (*api.ListIntegrationResp, error) {
	if reason, ok := validateIntegrationType(req.GetType()); !ok {
		return nil, status.Error(codes.InvalidArgument, reason)
	}

	order := req.GetOrder()
	if order == "" {
		order = defaultOrder
	}

	integrations, err := ig.IntegrationRepository.GetAllByType(ctx, req.GetType(), order)
	if err != nil {
		if errors.Is(err, database.ErrInvalidOrder) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Errorf(codes.Internal, "can't get integrations: %v", err)
	}

	return &api.ListIntegrationResp{
		Integrations: convert.IntegrationsToPB(integrations, true),
	}, nil
}

func (ig *IntegrationGeneric) TestAI(ctx context.Context, req *api.TestAIReq) (*emptypb.Empty, error) {
	if req.GetIntegration() == nil {
		return nil, status.Error(codes.InvalidArgument, "integration is nil")
	}

	if reason, ok := validateAIIntegration(req.GetIntegration()); !ok {
		return nil, status.Error(codes.InvalidArgument, reason)
	}

	integration, err := convert.IntegrationFromPB(req.GetIntegration())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "can't parse integration: %v", err)
	}

	aiIntegration, ok := integration.(*model.AI)
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "invalid integration type given: %T", integration)
	}

	client, err := aiclient.NewClient(aiIntegration)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "can't build ai client: %v", err)
	}

	if err := client.Test(ctx); err != nil {
		msg := fmt.Sprintf("ai integration is inaccessible: %v", err)
		return nil, errcommon.StatusWithReason(codes.InvalidArgument, IntegrationInaccessible, msg).Err()
	}

	return &emptypb.Empty{}, nil
}

func (ig *IntegrationGeneric) ExplainRuntimeEvent(ctx context.Context, req *api.ExplainRuntimeEventReq) (*api.ExplainRuntimeEventResp, error) {
	if req.GetIntegrationId() == "" {
		return nil, status.Error(codes.InvalidArgument, "integration ID is empty")
	}
	if req.GetEventId() == "" && req.GetEventJson() == "" {
		return nil, status.Error(codes.InvalidArgument, "event ID is empty")
	}
	if req.GetEventId() != "" {
		if _, err := uuid.Parse(req.GetEventId()); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "can't parse event ID: %v", err)
		}
	}

	eventJSON, err := ig.readRuntimeEventJSON(ctx, req.GetEventId(), req.GetEventJson())
	if err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetIntegrationId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "can't parse integration ID: %v", err)
	}

	integration, err := ig.IntegrationRepository.GetByTypeAndID(ctx, model.IntegrationAI, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "integration not found")
		}

		return nil, status.Errorf(codes.Internal, "can't get integration: %v", err)
	}

	aiIntegration, ok := integration.(*model.AI)
	if !ok {
		return nil, status.Errorf(codes.Internal, "invalid integration type given: %T", integration)
	}

	aiIntegration.DecryptSensitive(ig.Crypter)

	client, err := aiclient.NewClient(aiIntegration)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "can't build ai client: %v", err)
	}

	result, err := client.ExplainRuntimeEvent(ctx, req.GetEventId(), eventJSON)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "can't explain runtime event: %v", err)
	}

	return &api.ExplainRuntimeEventResp{
		Summary:       result.Summary,
		Risk:          result.Risk,
		PossibleCause: result.PossibleCause,
		NextSteps:     result.NextSteps,
		RawText:       result.RawText,
	}, nil
}

// readRuntimeEventJSON returns the event to analyse. It's read from History API
// by ID rather than taken from the request, so that what the model sees is the
// event the system actually recorded and not whatever a browser chose to send.
// The caller-supplied JSON is only a fallback for a deployment where History API
// is unreachable, and is dropped entirely once every client sends an event ID.
func (ig *IntegrationGeneric) readRuntimeEventJSON(ctx context.Context, eventID, fallbackJSON string) (string, error) {
	if eventID == "" || ig.RuntimeHistory == nil {
		if fallbackJSON == "" {
			return "", status.Error(codes.Internal, "runtime history is not configured")
		}

		return fallbackJSON, nil
	}

	event, err := ig.RuntimeHistory.Read(ctx, &history_api.ReadRuntimeEventReq{Id: eventID})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return "", status.Error(codes.NotFound, "runtime event not found")
		}

		if fallbackJSON == "" {
			return "", status.Errorf(codes.Internal, "can't get runtime event: %v", err)
		}

		log.Warn().Err(err).Str("event_id", eventID).Msg("Can't get runtime event, falling back to the event JSON from the request")

		return fallbackJSON, nil
	}

	// Same options the HTTP gateway marshals responses with, so that the model
	// reads the event in the shape the rest of the product presents it in.
	eventJSON, err := protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}.Marshal(event)
	if err != nil {
		return "", status.Errorf(codes.Internal, "can't marshal runtime event: %v", err)
	}

	return string(eventJSON), nil
}

func validateIntegration(req *api.Integration) (reason string, valid bool) {
	if req.GetName() == "" {
		return "name is empty", false
	}
	if req.GetConfig() == nil {
		return "config is nil", false
	}

	switch req.GetType() {
	case model.IntegrationEmail:
		conf, ok := req.GetConfig().(*api.Integration_Email)
		if !ok {
			return fmt.Sprintf("integration type %s does not match config type %T", req.GetType(), req.GetConfig()), false
		}

		reason, valid = validateEmail(conf.Email)
		if !valid {
			return
		}

	case model.IntegrationWebhook:
		conf, ok := req.GetConfig().(*api.Integration_Webhook)
		if !ok {
			return fmt.Sprintf("integration type %s does not match config type %T", req.GetType(), req.GetConfig()), false
		}

		reason, valid = validateWebhook(conf.Webhook)
		if !valid {
			return
		}

	case model.IntegrationSyslog:
		conf, ok := req.GetConfig().(*api.Integration_Syslog)
		if !ok {
			return fmt.Sprintf("integration type %s does not match config type %T", req.GetType(), req.GetConfig()), false
		}

		reason, valid = validateSyslog(conf.Syslog)
		if !valid {
			return
		}

	case model.IntegrationAI:
		conf, ok := req.GetConfig().(*api.Integration_Ai)
		if !ok {
			return fmt.Sprintf("integration type %s does not match config type %T", req.GetType(), req.GetConfig()), false
		}

		reason, valid = validateAI(conf.Ai)
		if !valid {
			return
		}

	default:
		return fmt.Sprintf("unsupported integration type given: %s", req.GetType()), false
	}

	return "", true
}

func validateIntegrationType(it string) (reason string, valid bool) {
	if it == "" {
		return "integration type is empty", false
	}
	if !model.IntegrationTypeSupported(it) {
		return fmt.Sprintf("unsupported integration type given: %s", it), false
	}
	return "", true
}

func validateEmail(e *api.Email) (reason string, valid bool) {
	if e.GetServer() == "" {
		return "server is empty", false
	}
	return "", true
}

func validateWebhook(w *api.Webhook) (reason string, valid bool) {
	if w.GetUrl() == "" {
		return "url is empty", false
	}
	return "", true
}

func validateSyslog(w *api.Syslog) (reason string, valid bool) {
	if w.GetAddress() == "" {
		return "address is empty", false
	}

	_, err := url.Parse(w.GetAddress())
	if err != nil {
		return fmt.Sprintf("can't parse address '%s': %+v", w.GetAddress(), err), false
	}

	return "", true
}

func validateAIIntegration(req *api.Integration) (reason string, valid bool) {
	if req.GetType() != model.IntegrationAI {
		return fmt.Sprintf("unsupported integration type given: %s", req.GetType()), false
	}

	conf, ok := req.GetConfig().(*api.Integration_Ai)
	if !ok {
		return fmt.Sprintf("integration type %s does not match config type %T", req.GetType(), req.GetConfig()), false
	}

	return validateAI(conf.Ai)
}

func validateAI(conf *api.AI) (reason string, valid bool) {
	if conf.GetModel() == "" {
		return "model is empty", false
	}

	switch conf.GetProvider() {
	case api.AI_PROVIDER_OPENAI_COMPATIBLE, api.AI_PROVIDER_ANTHROPIC, api.AI_PROVIDER_OLLAMA,
		api.AI_PROVIDER_QWEN, api.AI_PROVIDER_DEEPSEEK, api.AI_PROVIDER_GLM:
	default:
		return fmt.Sprintf("unsupported ai provider given: %s", conf.GetProvider().String()), false
	}

	if conf.GetBaseUrl() == "" {
		// Anthropic and Ollama have a single meaningful endpoint, but an empty
		// base url for an openai-compatible provider silently resolves to the
		// public OpenAI API, which nobody configures by leaving a field blank.
		if conf.GetProvider() == api.AI_PROVIDER_OPENAI_COMPATIBLE {
			return "base url is empty", false
		}
	} else {
		parsed, err := url.Parse(conf.GetBaseUrl())
		if err != nil {
			return fmt.Sprintf("can't parse base url '%s': %+v", conf.GetBaseUrl(), err), false
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return fmt.Sprintf("base url is invalid: %s", conf.GetBaseUrl()), false
		}
	}

	// The flag promises event data never leaves the deployment, so the endpoint
	// events would actually be posted to has to back that up. An empty base url
	// is checked too: it resolves to a provider default that may well be public.
	if conf.GetIsLocal() && !aiclient.IsLocalEndpoint(&model.AI{
		Provider: convert.AIProviderFromPB(conf.GetProvider()),
		BaseURL:  conf.GetBaseUrl(),
	}) {
		return "base url is not a local endpoint", false
	}

	return "", true
}

func idsFromNotifications(ns ...*model.Notification) []string {
	res := make([]string, 0, len(ns))
	for _, n := range ns {
		res = append(res, n.ID.String())
	}
	return res
}

func (ig *IntegrationGeneric) emailUpdateMap(name string, conf *api.Email) map[string]any {
	encryptedPassword := ""
	if conf.GetPassword() != "" {
		encryptedPassword = ig.Crypter.EncryptStringAsHex(conf.GetPassword())
	}

	return map[string]any{
		"Name":              name,
		"From":              conf.GetFrom(),
		"Server":            conf.GetServer(),
		"AuthType":          conf.GetAuthType(),
		"Username":          conf.GetUsername(),
		"EncryptedPassword": encryptedPassword,
		"UseTLS":            conf.GetUseTls(),
		"UseStartTLS":       conf.GetUseStartTls(),
		"Insecure":          conf.GetInsecure(),
		"CA":                conf.GetCa(),
	}
}

func (ig *IntegrationGeneric) webhookUpdateMap(name string, conf *api.Webhook) map[string]any {
	encryptedPassword := ""
	if conf.GetPassword() != "" {
		encryptedPassword = ig.Crypter.EncryptStringAsHex(conf.GetPassword())
	}

	return map[string]any{
		"Name":              name,
		"URL":               conf.GetUrl(),
		"Login":             conf.GetLogin(),
		"EncryptedPassword": encryptedPassword,
		"Insecure":          conf.GetInsecure(),
		"CA":                conf.GetCa(),
	}
}

func (ig *IntegrationGeneric) syslogUpdateMap(name string, conf *api.Syslog) map[string]any {
	return map[string]any{
		"Name":    name,
		"Address": conf.GetAddress(),
	}
}

func (ig *IntegrationGeneric) aiUpdateMap(name string, conf *api.AI) map[string]any {
	m := map[string]any{
		"Name":     name,
		"Provider": convert.AIProviderFromPB(conf.GetProvider()),
		"BaseURL":  conf.GetBaseUrl(),
		"Model":    conf.GetModel(),
		"IsLocal":  conf.GetIsLocal(),
		"Insecure": conf.GetInsecure(),
		"CA":       conf.GetCa(),
	}

	// The key is only ever handed out masked, so an edit that doesn't touch it
	// sends nothing back. Leaving the column out of the update map keeps the
	// stored key; writing "" here would disable the integration on any edit.
	if conf.GetApiKey() != "" {
		m["EncryptedAPIKey"] = ig.Crypter.EncryptStringAsHex(conf.GetApiKey())
	}

	return m
}

// restoreStoredAPIKey fills in the API key an update request left out, so that
// the connectivity check probes with the credentials the integration will
// actually keep using rather than with none at all.
func (ig *IntegrationGeneric) restoreStoredAPIKey(ctx context.Context, i model.Integration) error {
	ai, ok := i.(*model.AI)
	if !ok || ai.APIKey != "" || ai.ID == uuid.Nil {
		return nil
	}

	stored, err := ig.IntegrationRepository.GetByTypeAndID(ctx, model.IntegrationAI, ai.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}

		return err
	}

	storedAI, ok := stored.(*model.AI)
	if !ok {
		return nil
	}

	storedAI.DecryptSensitive(ig.Crypter)
	ai.APIKey = storedAI.APIKey

	return nil
}

func (ig *IntegrationGeneric) testIntegration(ctx context.Context, integration model.Integration) error {
	if aiIntegration, ok := integration.(*model.AI); ok {
		client, err := aiclient.NewClient(aiIntegration)
		if err != nil {
			return err
		}

		return client.Test(ctx)
	}

	n, err := notifier.FromIntegration(integration)
	if err != nil {
		return err
	}

	return n.Test(ctx)
}
