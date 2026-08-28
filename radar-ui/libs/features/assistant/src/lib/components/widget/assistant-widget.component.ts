import { Router } from '@angular/router';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import {
    AfterViewChecked,
    ChangeDetectionStrategy,
    Component,
    DestroyRef,
    ElementRef,
    Input,
    OnInit,
    ViewChild
} from '@angular/core';
import { Observable, combineLatest, map, take } from 'rxjs';

import { I18nService } from '@cs/i18n';
import {
    ASSISTANT_ATTACHMENT_ACCEPT,
    ASSISTANT_MAX_ATTACHMENTS,
    ASSISTANT_QUICK_ACTIONS,
    ASSISTANT_SUGGESTION_KEYS,
    ASSISTANT_TOUR_TOPICS,
    AssistantAttachment,
    AssistantMessage,
    AssistantMode,
    AssistantStoreService,
    AssistantView
} from '@cs/domains/assistant';
import { IntegrationAI, IntegrationStoreService } from '@cs/domains/integration';
import { LoadStatus, RouterName } from '@cs/core';

@Component({
    selector: 'cs-assistant-widget',
    templateUrl: './assistant-widget.component.html',
    styleUrl: './assistant-widget.component.scss',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AssistantFeatureWidgetComponent implements OnInit, AfterViewChecked {
    /**
     * Whether the signed-in user may read integrations. Without that permission
     * there is no AI integration to answer with and the assistant's own RPC
     * would be refused, so the widget neither loads them nor appears. The shell
     * passes it in: this library must not import the auth domain, which it and
     * @cs/core import each other through.
     */
    @Input() canReadIntegrations = false;

    @ViewChild('body') body?: ElementRef<HTMLElement>;

    readonly integrations$: Observable<IntegrationAI[]> = this.integrationStoreService.aiIntegrations$;

    /** The widget only exists once an AI integration does. */
    readonly isAvailable$: Observable<boolean> = this.integrations$.pipe(map((integrations) => !!integrations.length));

    readonly isOpen$: Observable<boolean> = this.assistantStoreService.isOpen$;

    readonly isStreaming$: Observable<boolean> = this.assistantStoreService.isStreaming$;

    readonly view$: Observable<AssistantView> = this.assistantStoreService.view$;

    readonly messages$: Observable<AssistantMessage[]> = this.assistantStoreService.messages$;

    readonly integrationId$: Observable<string> = this.assistantStoreService.integrationId$;

    readonly eventId$: Observable<string> = this.assistantStoreService.eventId$;

    readonly attachments$: Observable<AssistantAttachment[]> = this.assistantStoreService.pendingAttachments$;

    readonly assistantView = AssistantView;

    readonly quickActions = ASSISTANT_QUICK_ACTIONS;

    readonly tourTopics = ASSISTANT_TOUR_TOPICS;

    readonly suggestions = ASSISTANT_SUGGESTION_KEYS;

    readonly attachmentAccept = ASSISTANT_ATTACHMENT_ACCEPT;

    readonly maxAttachments = ASSISTANT_MAX_ATTACHMENTS;

    input = '';

    /**
     * What is typed into the collapsed widget. It is separate from the panel's
     * composer: a question started down there opens the panel, and the two
     * inputs are never on screen at the same time.
     */
    dockInput = '';

    private lastScrollHeight = 0;

    constructor(
        private readonly assistantStoreService: AssistantStoreService,
        private readonly destroyRef: DestroyRef,
        private readonly i18nService: I18nService,
        private readonly integrationStoreService: IntegrationStoreService,
        private readonly router: Router
    ) {}

    ngOnInit() {
        // The integrations page loads these through its route guard, and the
        // widget is on every page, so it asks for them once itself.
        if (this.canReadIntegrations) {
            this.integrationStoreService.loadStatus$
                .pipe(take(1), takeUntilDestroyed(this.destroyRef))
                .subscribe((status) => {
                    if (status === LoadStatus.INIT) {
                        this.integrationStoreService.load();
                    }
                });
        }

        // The widget is part of the shell, so it picks an integration itself
        // rather than making the user choose before the first question. The
        // choice stays theirs whenever there is more than one.
        combineLatest([this.integrations$, this.integrationId$])
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe(([integrations, integrationId]) => {
                const isKnown = integrations.some((integration) => integration.id === integrationId);

                if (!isKnown && integrations.length) {
                    this.assistantStoreService.selectIntegration(integrations[0].id);
                }
            });
    }

    ngAfterViewChecked() {
        this.scrollToLatest();
    }

    heading(view: AssistantView): string {
        return view === AssistantView.CHAT
            ? 'Assistant.Widget.Title.Chat'
            : view === AssistantView.TOUR
              ? 'Assistant.Widget.Title.Tour'
              : 'Assistant.Widget.Title.Home';
    }

    open() {
        this.assistantStoreService.open();
    }

    /** Asks the question typed into the collapsed widget, opening the panel. */
    sendFromDock() {
        const question = this.dockInput.trim();

        if (!question) {
            return;
        }

        this.assistantStoreService.open({ question });
        this.dockInput = '';
    }

    onDockKeyDown(event: KeyboardEvent) {
        if (event.key !== 'Enter') {
            return;
        }

        event.preventDefault();
        this.sendFromDock();
    }

    /**
     * Opens the interview that ends in a support request. The first message is
     * the user's own words for it, so the assistant has something to start from.
     */
    reportProblem() {
        this.assistantStoreService.open({
            question: this.i18nService.translate('Assistant.Widget.Support.FirstMessage'),
            mode: AssistantMode.SUPPORT
        });
    }

    close() {
        this.assistantStoreService.close();
    }

    showHome() {
        this.assistantStoreService.showView(AssistantView.HOME);
    }

    showTour() {
        this.assistantStoreService.showView(AssistantView.TOUR);
    }

    startChat() {
        this.assistantStoreService.startChat();
    }

    /**
     * Opens a new conversation with this question already asked. The key is
     * translated here rather than in the template: Angular does not allow a
     * pipe inside an event handler.
     */
    askKey(localizationKey: string) {
        this.assistantStoreService.startChat(this.i18nService.translate(localizationKey));
    }

    openChatsPage() {
        this.assistantStoreService.close();
        void this.router.navigate([RouterName.CHATS]);
    }

    selectIntegration(event: Event) {
        this.assistantStoreService.selectIntegration((event.target as HTMLSelectElement).value);
    }

    send() {
        const content = this.input.trim();

        if (!content) {
            return;
        }

        this.assistantStoreService.send(content);
        this.input = '';
    }

    attach(event: Event) {
        const picker = event.target as HTMLInputElement;

        if (picker.files?.length) {
            this.assistantStoreService.attach(Array.from(picker.files));
        }

        // Reset, so that picking the same file twice in a row still fires.
        picker.value = '';
    }

    removeAttachment(name: string) {
        this.assistantStoreService.removeAttachment(name);
    }

    /** Enter sends, Shift+Enter starts a new line. */
    onKeyDown(event: KeyboardEvent) {
        if (event.key !== 'Enter' || event.shiftKey) {
            return;
        }

        event.preventDefault();
        this.send();
    }

    /**
     * Keeps the newest text in view while an answer streams in, without
     * fighting a user who scrolled up to read something.
     */
    private scrollToLatest() {
        const element = this.body?.nativeElement;

        if (!element) {
            return;
        }

        const isAtBottom = element.scrollHeight - element.scrollTop - element.clientHeight < 80;

        if (element.scrollHeight !== this.lastScrollHeight && isAtBottom) {
            element.scrollTop = element.scrollHeight;
        }

        this.lastScrollHeight = element.scrollHeight;
    }
}
