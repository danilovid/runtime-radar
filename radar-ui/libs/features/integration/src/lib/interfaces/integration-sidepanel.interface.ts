import { Observable } from 'rxjs';

import { Notification } from '@cs/domains/notification';
import { RegisteredCluster } from '@cs/domains/cluster';
import {
    Integration,
    IntegrationAI,
    IntegrationEmail,
    IntegrationSyslog,
    IntegrationType,
    IntegrationWebhook
} from '@cs/domains/integration';

import {
    IntegrationAIForm,
    IntegrationEmailForm,
    IntegrationSyslogForm,
    IntegrationWebhookForm
} from './integration-form.interface';

export interface IntegrationSidepanelFormProps {
    type: IntegrationType;
    ai: IntegrationAI;
    email: IntegrationEmail;
    syslog: IntegrationSyslog;
    webhook: IntegrationWebhook;
    isEdit: boolean;
}

interface AbstractIntegrationSidepanelFormOutputs {
    type: IntegrationType;
    hasSkipCheck: boolean;
}

interface IntegrationSidepanelEmailFormOutputs extends AbstractIntegrationSidepanelFormOutputs {
    type: IntegrationType.EMAIL;
    email: IntegrationEmailForm;
}

interface IntegrationSidepanelAIFormOutputs extends AbstractIntegrationSidepanelFormOutputs {
    type: IntegrationType.AI;
    ai: IntegrationAIForm;
}

interface IntegrationSidepanelSyslogFormOutputs extends AbstractIntegrationSidepanelFormOutputs {
    type: IntegrationType.SYSLOG;
    syslog: IntegrationSyslogForm;
}

interface IntegrationSidepanelWebhookFormOutputs extends AbstractIntegrationSidepanelFormOutputs {
    type: IntegrationType.WEBHOOK;
    webhook: IntegrationWebhookForm;
}

export type IntegrationSidepanelFormOutputs =
    | IntegrationSidepanelAIFormOutputs
    | IntegrationSidepanelEmailFormOutputs
    | IntegrationSidepanelSyslogFormOutputs
    | IntegrationSidepanelWebhookFormOutputs;

export interface IntegrationSidepanelRecipientFormProps {
    centralUrl$: Observable<string>;
    activeRegisteredCluster$: Observable<RegisteredCluster | undefined>;
    integration: Integration;
    notification?: Notification;
    isEdit?: boolean;
}
