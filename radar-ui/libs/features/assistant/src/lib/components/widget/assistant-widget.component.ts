import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import {
    AfterViewChecked,
    ChangeDetectionStrategy,
    Component,
    DestroyRef,
    ElementRef,
    OnInit,
    ViewChild
} from '@angular/core';
import { Observable, combineLatest, map } from 'rxjs';

import { AssistantMessage, AssistantStoreService } from '@cs/domains/assistant';
import { IntegrationAI, IntegrationStoreService } from '@cs/domains/integration';

@Component({
    selector: 'cs-assistant-widget',
    templateUrl: './assistant-widget.component.html',
    styleUrl: './assistant-widget.component.scss',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AssistantFeatureWidgetComponent implements OnInit, AfterViewChecked {
    @ViewChild('thread') thread?: ElementRef<HTMLElement>;

    readonly integrations$: Observable<IntegrationAI[]> = this.integrationStoreService.aiIntegrations$;

    /** The launcher only exists once an AI integration does. */
    readonly isAvailable$: Observable<boolean> = this.integrations$.pipe(map((integrations) => !!integrations.length));

    readonly isOpen$: Observable<boolean> = this.assistantStoreService.isOpen$;

    readonly isStreaming$: Observable<boolean> = this.assistantStoreService.isStreaming$;

    readonly messages$: Observable<AssistantMessage[]> = this.assistantStoreService.messages$;

    readonly integrationId$: Observable<string> = this.assistantStoreService.integrationId$;

    readonly eventId$: Observable<string> = this.assistantStoreService.eventId$;

    input = '';

    private lastRenderedLength = 0;

    constructor(
        private readonly assistantStoreService: AssistantStoreService,
        private readonly destroyRef: DestroyRef,
        private readonly integrationStoreService: IntegrationStoreService
    ) {}

    ngOnInit() {
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

    open() {
        this.assistantStoreService.open();
    }

    close() {
        this.assistantStoreService.close();
    }

    clear() {
        this.assistantStoreService.clear();
    }

    selectIntegration(integrationId: string) {
        this.assistantStoreService.selectIntegration(integrationId);
    }

    send() {
        const content = this.input.trim();

        if (!content) {
            return;
        }

        this.assistantStoreService.send(content);
        this.input = '';
    }

    /** Enter sends, Shift+Enter starts a new line. */
    onKeyDown(event: KeyboardEvent) {
        if (event.key !== 'Enter' || event.shiftKey) {
            return;
        }

        event.preventDefault();
        this.send();
    }

    trackMessage(_: number, message: AssistantMessage): string {
        return message.id;
    }

    /**
     * Keeps the newest text in view while an answer streams in, without
     * fighting a user who scrolled up to read something.
     */
    private scrollToLatest() {
        const element = this.thread?.nativeElement;

        if (!element) {
            return;
        }

        const isAtBottom = element.scrollHeight - element.scrollTop - element.clientHeight < 80;

        if (element.scrollHeight !== this.lastRenderedLength && isAtBottom) {
            element.scrollTop = element.scrollHeight;
        }

        this.lastRenderedLength = element.scrollHeight;
    }
}
