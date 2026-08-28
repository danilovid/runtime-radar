import { ChangeDetectionStrategy, ChangeDetectorRef, Component, Inject, Input } from '@angular/core';

import {
    AssistantAction,
    AssistantActionState,
    AssistantMessage,
    AssistantRole,
    AssistantStoreService,
    AssistantSupportRequest,
    extractSupportRequest,
    supportMailtoLink
} from '@cs/domains/assistant';
import { CoreWindowService, SUPPORT_EMAIL } from '@cs/core';

@Component({
    selector: 'cs-assistant-message',
    templateUrl: './assistant-message.component.html',
    styleUrl: './assistant-message.component.scss',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AssistantFeatureMessageComponent {
    @Input({ required: true }) message!: AssistantMessage;

    readonly assistantRole = AssistantRole;

    readonly actionState = AssistantActionState;

    /** The secret the user has just copied, so the button can say so. */
    copiedSecret = '';

    /**
     * The answer is split once per version of the text rather than on every
     * change detection run: an answer being streamed changes many times a
     * second, and this runs for every message on screen.
     */
    private parsed: { source: string; content: string; request?: AssistantSupportRequest } = {
        source: '',
        content: ''
    };

    constructor(
        private readonly assistantStoreService: AssistantStoreService,
        private readonly changeDetectorRef: ChangeDetectorRef,
        private readonly coreWindowService: CoreWindowService,
        @Inject(SUPPORT_EMAIL) private readonly supportEmail: string
    ) {}

    /**
     * The answer without the support request, which is rendered as a card of
     * its own further down.
     */
    get content(): string {
        return this.split().content;
    }

    get supportRequest(): AssistantSupportRequest | undefined {
        return this.split().request;
    }

    get mailto(): string {
        const request = this.supportRequest;

        return request ? supportMailtoLink(this.supportEmail, request) : '';
    }

    get supportAddress(): string {
        return this.supportEmail;
    }

    confirm(action: AssistantAction) {
        this.assistantStoreService.confirmAction(this.message.id, action.id);
    }

    decline() {
        this.assistantStoreService.declineAction(this.message.id);
    }

    /**
     * Copies a secret. It is shown once and cannot be read again, so this is
     * the only way to keep it that does not involve retyping.
     */
    copy(value: string) {
        void this.coreWindowService.navigator.clipboard.writeText(value).then(() => {
            this.copiedSecret = value;
            this.changeDetectorRef.markForCheck();
        });
    }

    private split(): { content: string; request?: AssistantSupportRequest } {
        if (this.parsed.source !== this.message.content) {
            this.parsed = { source: this.message.content, ...extractSupportRequest(this.message.content) };
        }

        return this.parsed;
    }
}
