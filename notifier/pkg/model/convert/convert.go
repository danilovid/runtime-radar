package convert

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/runtime-radar/runtime-radar/notifier/api"
	"github.com/runtime-radar/runtime-radar/notifier/pkg/model"
)

func IntegrationsToPB(mis []model.Integration, maskSensitive bool) []*api.Integration {
	pbis := make([]*api.Integration, 0, len(mis))
	for _, mi := range mis {
		pbis = append(pbis, IntegrationToPB(mi, maskSensitive))
	}
	return pbis
}

func IntegrationToPB(mi model.Integration, maskSensitive bool) *api.Integration {
	if maskSensitive {
		mi.MaskSensitive()
	}

	switch i := mi.(type) {
	case *model.Email:
		return &api.Integration{
			Id:   i.ID.String(),
			Name: i.Name,
			Type: model.IntegrationEmail,
			Config: &api.Integration_Email{
				Email: EmailToPB(i),
			},
		}

	case *model.Webhook:
		return &api.Integration{
			Id:   i.ID.String(),
			Name: i.Name,
			Type: model.IntegrationWebhook,
			Config: &api.Integration_Webhook{
				Webhook: WebhookToPB(i),
			},
		}
	case *model.Syslog:
		return &api.Integration{
			Id:   i.ID.String(),
			Name: i.Name,
			Type: model.IntegrationSyslog,
			Config: &api.Integration_Syslog{
				Syslog: SyslogToPB(i),
			},
		}
	case *model.AI:
		return &api.Integration{
			Id:   i.ID.String(),
			Name: i.Name,
			Type: model.IntegrationAI,
			Config: &api.Integration_Ai{
				Ai: AIToPB(i),
			},
		}
	default:
		panic(fmt.Sprintf("unsupported integration type: %T", i)) // normally should not happen
	}
}

func NotificationToPB(n *model.Notification) *api.Notification {
	pbn := api.Notification{
		Id:              n.ID.String(),
		Name:            n.Name,
		IntegrationType: n.IntegrationType,
		IntegrationId:   n.IntegrationID.String(),
		Template:        n.Template,
		Recipients:      n.Recipients,
		EventType:       n.EventType,
		CentralCsUrl:    n.CentralCSURL,
		CsClusterId:     n.CSClusterID,
		CsClusterName:   n.CSClusterName,
		OwnCsUrl:        n.OwnCSURL,
	}

	configsCount := 0

	if n.EmailConfig != nil {
		pbn.Config = &api.Notification_Email{
			Email: (*api.EmailConfig)(n.EmailConfig),
		}
		configsCount++
	}
	if n.WebhookConfig != nil {
		pbn.Config = &api.Notification_Webhook{
			Webhook: (*api.WebhookConfig)(n.WebhookConfig),
		}
		configsCount++
	}
	if n.SyslogConfig != nil {
		pbn.Config = &api.Notification_Syslog{
			Syslog: (*api.SyslogConfig)(n.SyslogConfig),
		}
		configsCount++
	}

	// This normally should not happen.
	// Only one config field should be set at a time according to API specification.
	// This case can only happen if model.Notification was created directly at a storage without any validation.
	if configsCount > 1 {
		panic(fmt.Sprintf("expected 1 config to be set, %d given", configsCount))
	}

	return &pbn
}

func NotificationsToPB(notifications []*model.Notification) []*api.Notification {
	pbNotifications := make([]*api.Notification, len(notifications))
	for i, n := range notifications {
		pbNotifications[i] = NotificationToPB(n)
	}
	return pbNotifications
}

func EmailToPB(e *model.Email) *api.Email {
	return &api.Email{
		From:        e.From,
		Server:      e.Server,
		Username:    e.Username,
		Password:    e.EncryptedPassword,
		UseTls:      e.UseTLS,
		UseStartTls: e.UseStartTLS,
		Insecure:    e.Insecure,
		AuthType:    EmailAuthTypeToPB(e.AuthType),
		Ca:          e.CA,
	}
}

func WebhookToPB(w *model.Webhook) *api.Webhook {
	return &api.Webhook{
		Url:      w.URL,
		Login:    w.Login,
		Password: w.EncryptedPassword,
		Insecure: w.Insecure,
		Ca:       w.CA,
	}
}

func SyslogToPB(w *model.Syslog) *api.Syslog {
	return &api.Syslog{
		Address: w.Address,
	}
}

func AIToPB(a *model.AI) *api.AI {
	return &api.AI{
		Provider: AIProviderToPB(a.Provider),
		BaseUrl:  a.BaseURL,
		Model:    a.Model,
		ApiKey:   a.EncryptedAPIKey,
		IsLocal:  a.IsLocal,
		Insecure: a.Insecure,
		Ca:       a.CA,
	}
}

func EmailAuthTypeFromPB(proto api.Email_AuthType) model.EmailAuthType {
	switch proto {
	case api.Email_AUTH_TYPE_PLAIN:
		return model.AuthTypePlain
	case api.Email_AUTH_TYPE_LOGIN:
		return model.AuthTypeLogin
	case api.Email_AUTH_TYPE_CRAM_MD5:
		return model.AuthTypeCramMD5
	default:
		return model.AuthTypeNone
	}
}

func EmailAuthTypeToPB(at model.EmailAuthType) api.Email_AuthType {
	switch at {
	case model.AuthTypePlain:
		return api.Email_AUTH_TYPE_PLAIN
	case model.AuthTypeLogin:
		return api.Email_AUTH_TYPE_LOGIN
	case model.AuthTypeCramMD5:
		return api.Email_AUTH_TYPE_CRAM_MD5
	default:
		return api.Email_AUTH_TYPE_NONE
	}
}

func AIProviderFromPB(provider api.AI_Provider) model.AIProvider {
	switch provider {
	case api.AI_PROVIDER_ANTHROPIC:
		return model.AIProviderAnthropic
	case api.AI_PROVIDER_OLLAMA:
		return model.AIProviderOllama
	case api.AI_PROVIDER_QWEN:
		return model.AIProviderQwen
	case api.AI_PROVIDER_DEEPSEEK:
		return model.AIProviderDeepSeek
	case api.AI_PROVIDER_GLM:
		return model.AIProviderGLM
	default:
		return model.AIProviderOpenAICompatible
	}
}

func AIProviderToPB(provider model.AIProvider) api.AI_Provider {
	switch provider {
	case model.AIProviderAnthropic:
		return api.AI_PROVIDER_ANTHROPIC
	case model.AIProviderOllama:
		return api.AI_PROVIDER_OLLAMA
	case model.AIProviderQwen:
		return api.AI_PROVIDER_QWEN
	case model.AIProviderDeepSeek:
		return api.AI_PROVIDER_DEEPSEEK
	case model.AIProviderGLM:
		return api.AI_PROVIDER_GLM
	default:
		return api.AI_PROVIDER_OPENAI_COMPATIBLE
	}
}

func IntegrationFromPB(req *api.Integration) (model.Integration, error) {
	var id uuid.UUID
	var err error

	if req.GetId() != "" {
		id, err = uuid.Parse(req.GetId())
		if err != nil {
			return nil, fmt.Errorf("can't parse ID: %w", err)
		}
	}

	switch conf := req.GetConfig().(type) {
	case *api.Integration_Email:
		emailConf := conf.Email

		return &model.Email{
			Base:        model.Base{ID: id},
			Name:        req.GetName(),
			From:        emailConf.GetFrom(),
			Server:      emailConf.GetServer(),
			AuthType:    EmailAuthTypeFromPB(emailConf.GetAuthType()),
			Username:    emailConf.GetUsername(),
			Password:    emailConf.GetPassword(),
			UseTLS:      emailConf.GetUseTls(),
			UseStartTLS: emailConf.GetUseStartTls(),
			Insecure:    emailConf.GetInsecure(),
			CA:          emailConf.GetCa(),
		}, nil

	case *api.Integration_Webhook:
		webhookConf := conf.Webhook

		return &model.Webhook{
			Base:     model.Base{ID: id},
			Name:     req.GetName(),
			URL:      webhookConf.GetUrl(),
			Login:    webhookConf.GetLogin(),
			Password: webhookConf.GetPassword(),
			Insecure: webhookConf.GetInsecure(),
			CA:       webhookConf.GetCa(),
		}, nil

	case *api.Integration_Syslog:
		syslogConf := conf.Syslog

		return &model.Syslog{
			Base:    model.Base{ID: id},
			Name:    req.GetName(),
			Address: syslogConf.GetAddress(),
		}, nil
	case *api.Integration_Ai:
		aiConf := conf.Ai

		return &model.AI{
			Base:     model.Base{ID: id},
			Name:     req.GetName(),
			Provider: AIProviderFromPB(aiConf.GetProvider()),
			BaseURL:  aiConf.GetBaseUrl(),
			Model:    aiConf.GetModel(),
			APIKey:   aiConf.GetApiKey(),
			IsLocal:  aiConf.GetIsLocal(),
			Insecure: aiConf.GetInsecure(),
			CA:       aiConf.GetCa(),
		}, nil
	default:
		return nil, errors.New("can't parse integration type")
	}
}
