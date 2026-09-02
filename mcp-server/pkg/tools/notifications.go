package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/runtime-radar/runtime-radar/mcp-server/pkg/auth"
	notifier_api "github.com/runtime-radar/runtime-radar/notifier/api"
)

// maxNotifications bounds a listing the way every other listing here is
// bounded: an answer the model cannot read is worse than a truncated one.
const maxNotifications = 50

// notificationServiceTypes are the transports a message can go out through.
// Notifier lists integrations one type at a time and refuses a request without
// one, so a full listing is the four of them put together.
var notificationServiceTypes = []string{"email", "webhook", "syslog", "ai"}

// runtimeEventType is the only kind of event a template can deliver. Admission
// findings have no template of their own yet.
const runtimeEventType = "runtime_event"

// NotificationService is one place messages go out through. Its credentials are
// deliberately absent: an SMTP password or an API key has no business being in
// a model's context, and nothing the assistant does with a template needs them.
type NotificationService struct {
	ID   string `json:"id" jsonschema:"identifier of the notification service, which create_notification_template takes"`
	Name string `json:"name" jsonschema:"name of the service, as shown in the UI"`
	Type string `json:"type" jsonschema:"how it delivers: email, webhook, syslog or ai"`
}

// ListNotificationServicesResult is the answer of list_notification_services.
type ListNotificationServicesResult struct {
	Services []NotificationService `json:"services" jsonschema:"the notification services configured in the product"`
	Note     string                `json:"note,omitempty" jsonschema:"present when the list was cut short"`
}

// NotificationTemplate is what turns a detected threat into a message.
type NotificationTemplate struct {
	ID         string   `json:"id" jsonschema:"identifier of the template"`
	Name       string   `json:"name" jsonschema:"name of the template, as shown in the UI"`
	ServiceID  string   `json:"service_id" jsonschema:"identifier of the notification service it sends through"`
	Type       string   `json:"type" jsonschema:"how it delivers: email, webhook, syslog or ai"`
	EventType  string   `json:"event_type" jsonschema:"the kind of event it reports, for example runtime_event"`
	Recipients []string `json:"recipients,omitempty" jsonschema:"who receives it: addresses for email, empty for the transports that have a single destination"`
}

// ListNotificationTemplatesResult is the answer of list_notification_templates.
type ListNotificationTemplatesResult struct {
	Templates []NotificationTemplate `json:"templates" jsonschema:"the notification templates configured in the product"`
	Note      string                 `json:"note,omitempty" jsonschema:"present when the list was cut short"`
}

// CreateNotificationServiceArgs are the arguments of create_notification_service.
// Only the transports that need no credential are here; see the tool's
// description for why email is not among them.
type CreateNotificationServiceArgs struct {
	Type     string `json:"type" jsonschema:"how it delivers: syslog or webhook. Required"`
	Name     string `json:"name" jsonschema:"name of the service, shown in the UI. Required"`
	Address  string `json:"address,omitempty" jsonschema:"syslog only: the server, for example tcp://192.0.2.1:514 or udp://logs.example.com:514"`
	URL      string `json:"url,omitempty" jsonschema:"webhook only: the address messages are posted to"`
	Insecure bool   `json:"insecure,omitempty" jsonschema:"webhook only: do not verify the TLS certificate of the endpoint. Leave false unless the user asked for it"`
}

// CreateNotificationServiceResult is the answer of create_notification_service.
type CreateNotificationServiceResult struct {
	ID   string `json:"id" jsonschema:"identifier of the service that was created, which create_notification_template takes"`
	Name string `json:"name" jsonschema:"name of the service that was created"`
	Note string `json:"note" jsonschema:"what the user should do now that the service exists"`
}

// CreateNotificationTemplateArgs are the arguments of create_notification_template.
type CreateNotificationTemplateArgs struct {
	Name       string   `json:"name" jsonschema:"name of the template, shown in the UI. Required"`
	ServiceID  string   `json:"service_id" jsonschema:"identifier of the notification service to send through, as returned by list_notification_services. Required, and it cannot be guessed from a name"`
	EventType  string   `json:"event_type" jsonschema:"the kind of event to report. The only value Runtime Radar accepts today is runtime_event; findings of admission control cannot be delivered by a template yet. Required"`
	Recipients []string `json:"recipients,omitempty" jsonschema:"who receives the message. Required for email, ignored by transports with a single destination"`
	Template   string   `json:"template,omitempty" jsonschema:"body of the message. Leave empty to use the product's default for this event type"`
	Subject    string   `json:"subject,omitempty" jsonschema:"subject line, email only"`
}

// CreateNotificationTemplateResult is the answer of create_notification_template.
type CreateNotificationTemplateResult struct {
	ID   string `json:"id" jsonschema:"identifier of the template that was created"`
	Name string `json:"name" jsonschema:"name of the template that was created"`
	Note string `json:"note" jsonschema:"what the user should check now that the template exists"`
}

func registerNotificationTools(server *mcp.Server, deps *Deps) {
	addTool(server, deps, &mcp.Tool{
		Name:        "list_notification_services",
		Annotations: readOnly("List notification services"),
		Description: "List the services notifications are delivered through: email, webhook, syslog and AI. " +
			"Credentials are never returned. Use it to learn the identifiers create_notification_template takes, " +
			"and to tell the user which services exist before proposing a template.",
	}, []auth.Permission{auth.ReadIntegrations()}, listNotificationServices(deps))

	addTool(server, deps, &mcp.Tool{
		Name:        "list_notification_templates",
		Annotations: readOnly("List notification templates"),
		Description: "List the notification templates of the product: what each one reports, which service it " +
			"goes out through and who receives it. Use it to see whether a template already covers what the user " +
			"wants before proposing another one.",
	}, []auth.Permission{auth.ReadNotifications()}, listNotificationTemplates(deps))

	addTool(server, deps, &mcp.Tool{
		Name:        "create_notification_service",
		Annotations: write("Connect a notification service", false),
		Description: "Connect a service for notifications to go out through. Only syslog and webhook can be " +
			"connected this way, because they need no credential. Email and AI are deliberately not offered: their " +
			"configuration is a password or an API key, and anything given to this tool travels through the model " +
			"and stays in the conversation. Tell the user to add those two in the interface instead. A webhook " +
			"whose endpoint needs an authorization header is the same case: create it here without one and let " +
			"them add the header in the interface.",
	}, []auth.Permission{auth.CreateIntegrations()}, createNotificationService(deps))

	addTool(server, deps, &mcp.Tool{
		Name:        "create_notification_template",
		Annotations: write("Create a notification template", false),
		Description: "Create a notification template, which is what a policy rule sends when it fires. It needs " +
			"an existing notification service: this tool cannot create one, because that would mean handing " +
			"credentials to a model, and the user has to add the service in the interface first. A template " +
			"starts sending as soon as a rule references it, so state the service, the event type and the " +
			"recipients to the user and get their agreement before calling this.",
	}, []auth.Permission{auth.CreateNotifications()}, createNotificationTemplate(deps))
}

func listNotificationServices(deps *Deps) func(context.Context, struct{}) (ListNotificationServicesResult, error) {
	return func(ctx context.Context, _ struct{}) (ListNotificationServicesResult, error) {
		services, err := listServices(ctx, deps)
		if err != nil {
			return ListNotificationServicesResult{}, err
		}

		result := ListNotificationServicesResult{Services: services}
		if len(services) > maxNotifications {
			result.Services = services[:maxNotifications]
			result.Note = fmt.Sprintf("only the first %d services are listed", maxNotifications)
		}

		return result, nil
	}
}

func listNotificationTemplates(deps *Deps) func(context.Context, struct{}) (ListNotificationTemplatesResult, error) {
	return func(ctx context.Context, _ struct{}) (ListNotificationTemplatesResult, error) {
		resp, err := deps.Clients.Notifications.List(ctx, &notifier_api.ListNotificationReq{})
		if err != nil {
			return ListNotificationTemplatesResult{}, fmt.Errorf("can't list notification templates: %w", err)
		}

		notifications := resp.GetNotifications()
		result := ListNotificationTemplatesResult{Templates: make([]NotificationTemplate, 0, len(notifications))}

		for i, notification := range notifications {
			if i == maxNotifications {
				result.Note = fmt.Sprintf("only the first %d templates are listed", maxNotifications)

				break
			}

			result.Templates = append(result.Templates, NotificationTemplate{
				ID:         notification.GetId(),
				Name:       notification.GetName(),
				ServiceID:  notification.GetIntegrationId(),
				Type:       notification.GetIntegrationType(),
				EventType:  notification.GetEventType(),
				Recipients: notification.GetRecipients(),
			})
		}

		return result, nil
	}
}

func createNotificationTemplate(
	deps *Deps,
) func(context.Context, CreateNotificationTemplateArgs) (CreateNotificationTemplateResult, error) {
	return func(ctx context.Context, args CreateNotificationTemplateArgs) (CreateNotificationTemplateResult, error) {
		name := strings.TrimSpace(args.Name)
		if name == "" {
			return CreateNotificationTemplateResult{}, fmt.Errorf("name is required")
		}

		serviceID := strings.TrimSpace(args.ServiceID)
		if serviceID == "" {
			return CreateNotificationTemplateResult{}, fmt.Errorf(
				"service_id is required: read it from list_notification_services")
		}

		eventType := strings.TrimSpace(args.EventType)
		if eventType == "" {
			return CreateNotificationTemplateResult{}, fmt.Errorf("event_type is required")
		}

		// Checked here so that the model is told what to send instead of being
		// handed a gRPC error it can only repeat.
		if eventType != runtimeEventType {
			return CreateNotificationTemplateResult{}, fmt.Errorf(
				"event_type %q is not delivered by templates: the only value accepted is %s",
				eventType, runtimeEventType)
		}

		// The service decides the transport, so it is read back rather than
		// taken from the caller: a template whose type does not match the
		// service it points at is rejected further down anyway.
		serviceType, err := notificationServiceType(ctx, deps, serviceID)
		if err != nil {
			return CreateNotificationTemplateResult{}, err
		}

		req := &notifier_api.Notification{
			Name:            name,
			IntegrationId:   serviceID,
			IntegrationType: serviceType,
			EventType:       eventType,
			Recipients:      args.Recipients,
			Template:        args.Template,
		}

		if subject := strings.TrimSpace(args.Subject); subject != "" {
			req.Config = &notifier_api.Notification_Email{
				Email: &notifier_api.EmailConfig{SubjectTemplate: subject},
			}
		}

		resp, err := deps.Clients.Notifications.Create(ctx, req)
		if err != nil {
			return CreateNotificationTemplateResult{}, fmt.Errorf("can't create notification template: %w", err)
		}

		return CreateNotificationTemplateResult{
			ID:   resp.GetId(),
			Name: name,
			Note: "the template only sends once a policy rule references it: check the rules that should use it",
		}, nil
	}
}

// listServices reads every transport's integrations and returns them as one
// list, without their credentials.
func listServices(ctx context.Context, deps *Deps) ([]NotificationService, error) {
	services := make([]NotificationService, 0, len(notificationServiceTypes))

	for _, serviceType := range notificationServiceTypes {
		resp, err := deps.Clients.Integrations.List(ctx, &notifier_api.ListIntegrationReq{Type: serviceType})
		if err != nil {
			return nil, fmt.Errorf("can't list %s notification services: %w", serviceType, err)
		}

		for _, integration := range resp.GetIntegrations() {
			services = append(services, NotificationService{
				ID:   integration.GetId(),
				Name: integration.GetName(),
				Type: serviceType,
			})
		}
	}

	return services, nil
}

// notificationServiceType returns the transport of the service a template is
// being attached to, and fails when no such service exists, so that the user is
// told that rather than left with a template pointing nowhere.
func notificationServiceType(ctx context.Context, deps *Deps, serviceID string) (string, error) {
	services, err := listServices(ctx, deps)
	if err != nil {
		return "", err
	}

	for _, service := range services {
		if service.ID == serviceID {
			return service.Type, nil
		}
	}

	return "", fmt.Errorf("no notification service with id %s: read the identifiers from list_notification_services", serviceID)
}

func createNotificationService(
	deps *Deps,
) func(context.Context, CreateNotificationServiceArgs) (CreateNotificationServiceResult, error) {
	return func(ctx context.Context, args CreateNotificationServiceArgs) (CreateNotificationServiceResult, error) {
		name := strings.TrimSpace(args.Name)
		if name == "" {
			return CreateNotificationServiceResult{}, fmt.Errorf("name is required")
		}

		req := &notifier_api.Integration{Name: name, Type: strings.TrimSpace(args.Type)}

		switch req.GetType() {
		case "syslog":
			address := strings.TrimSpace(args.Address)
			if address == "" {
				return CreateNotificationServiceResult{}, fmt.Errorf("address is required for a syslog service")
			}

			req.Config = &notifier_api.Integration_Syslog{Syslog: &notifier_api.Syslog{Address: address}}
		case "webhook":
			url := strings.TrimSpace(args.URL)
			if url == "" {
				return CreateNotificationServiceResult{}, fmt.Errorf("url is required for a webhook service")
			}

			req.Config = &notifier_api.Integration_Webhook{
				Webhook: &notifier_api.Webhook{Url: url, Insecure: args.Insecure},
			}
		case "email", "ai":
			// Refused rather than attempted: the fields that are missing are a
			// password and an API key, and the way to supply them is not a chat.
			return CreateNotificationServiceResult{}, fmt.Errorf(
				"a %s service is configured with a credential and cannot be connected from a chat: "+
					"the user adds it in the interface, under notification services", req.GetType())
		default:
			return CreateNotificationServiceResult{}, fmt.Errorf(
				"unknown service type %q: syslog or webhook", req.GetType())
		}

		resp, err := deps.Clients.Integrations.Create(ctx, req)
		if err != nil {
			return CreateNotificationServiceResult{}, fmt.Errorf("can't create notification service: %w", err)
		}

		return CreateNotificationServiceResult{
			ID:   resp.GetId(),
			Name: name,
			Note: "nothing is sent through it until a notification template points at it",
		}, nil
	}
}
