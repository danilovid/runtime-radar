import {
    IntegrationAIProviderType,
    IntegrationAIProviderTypeOption,
    IntegrationEmailAuthType,
    IntegrationEmailAuthTypeOption,
    IntegrationType,
    IntegrationTypeOption
} from '../interfaces';

export const INTEGRATION_TYPE: IntegrationTypeOption[] = [
    {
        id: IntegrationType.EMAIL,
        localizationKey: 'Integration.Pseudo.Type.Email'
    },
    {
        id: IntegrationType.SYSLOG,
        localizationKey: 'Integration.Pseudo.Type.Syslog'
    },
    {
        id: IntegrationType.WEBHOOK,
        localizationKey: 'Integration.Pseudo.Type.Webhook'
    },
    {
        id: IntegrationType.AI,
        localizationKey: 'Integration.Pseudo.Type.Ai'
    }
];

export const INTEGRATION_EMAIL_AUTH_TYPE: IntegrationEmailAuthTypeOption[] = [
    {
        id: IntegrationEmailAuthType.NONE,
        localizationKey: 'Integration.Pseudo.AuthType.None'
    },
    {
        id: IntegrationEmailAuthType.CRAM_MD5,
        localizationKey: 'Integration.Pseudo.AuthType.Crammd5'
    },
    {
        id: IntegrationEmailAuthType.LOGIN,
        localizationKey: 'Integration.Pseudo.AuthType.Login'
    },
    {
        id: IntegrationEmailAuthType.PLAIN,
        localizationKey: 'Integration.Pseudo.AuthType.Plain'
    }
];

export const INTEGRATION_AI_PROVIDER_TYPE: IntegrationAIProviderTypeOption[] = [
    {
        id: IntegrationAIProviderType.OPENAI_COMPATIBLE,
        localizationKey: 'Integration.Pseudo.AIProvider.OpenAICompatible'
    },
    {
        id: IntegrationAIProviderType.ANTHROPIC,
        localizationKey: 'Integration.Pseudo.AIProvider.Anthropic'
    },
    {
        id: IntegrationAIProviderType.OLLAMA,
        localizationKey: 'Integration.Pseudo.AIProvider.Ollama'
    }
];
