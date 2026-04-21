package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/google/uuid"
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
	"google.golang.org/protobuf/types/known/emptypb"
	"gorm.io/gorm"
)

type IntegrationGeneric struct {
	api.UnimplementedIntegrationControllerServer

	IntegrationRepository  database.IntegrationRepository
	NotificationRepository database.NotificationRepository
	RuleController         enforcer_api.RuleControllerClient
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
	if req.GetEventJson() == "" {
		return nil, status.Error(codes.InvalidArgument, "event json is empty")
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

	result, err := client.ExplainRuntimeEvent(ctx, req.GetEventId(), req.GetEventJson())
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
	case api.AI_PROVIDER_OPENAI_COMPATIBLE, api.AI_PROVIDER_ANTHROPIC, api.AI_PROVIDER_OLLAMA:
	default:
		return fmt.Sprintf("unsupported ai provider given: %s", conf.GetProvider().String()), false
	}

	if conf.GetBaseUrl() == "" {
		return "", true
	}

	parsed, err := url.Parse(conf.GetBaseUrl())
	if err != nil {
		return fmt.Sprintf("can't parse base url '%s': %+v", conf.GetBaseUrl(), err), false
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Sprintf("base url is invalid: %s", conf.GetBaseUrl()), false
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
	encryptedAPIKey := ""
	if conf.GetApiKey() != "" {
		encryptedAPIKey = ig.Crypter.EncryptStringAsHex(conf.GetApiKey())
	}

	return map[string]any{
		"Name":            name,
		"Provider":        convert.AIProviderFromPB(conf.GetProvider()),
		"BaseURL":         conf.GetBaseUrl(),
		"Model":           conf.GetModel(),
		"EncryptedAPIKey": encryptedAPIKey,
		"IsLocal":         conf.GetIsLocal(),
		"Insecure":        conf.GetInsecure(),
		"CA":              conf.GetCa(),
	}
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
