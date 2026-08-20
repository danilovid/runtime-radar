import { Injectable } from '@angular/core';
import { Observable } from 'rxjs';

import { ApiPathService } from '@cs/api';
import { AuthTokenName } from '@cs/domains/auth';
import { CoreWindowService } from '@cs/core';

import { AssistantChatChunk, AssistantChatRequest, AssistantChatStreamEnvelope } from '../interfaces';

const ASSISTANT_CHAT_PATH = 'assistant/chat';

/**
 * The assistant answers as a stream, so this one request cannot go through
 * HttpClient: Angular's XHR backend only hands the body over once it is
 * complete. fetch gives a readable stream instead, which is what lets the
 * widget show the answer and the tool activity while the assistant is still
 * working.
 */
@Injectable({
    providedIn: 'root'
})
export class AssistantRequestService {
    constructor(
        private readonly apiPathService: ApiPathService,
        private readonly coreWindowService: CoreWindowService
    ) {}

    /**
     * Streams the answer to one question. Unsubscribing aborts the request,
     * which is what stops the assistant when the widget is closed.
     */
    chat(request: AssistantChatRequest): Observable<AssistantChatChunk> {
        return new Observable<AssistantChatChunk>((subscriber) => {
            const controller = new AbortController();

            this.read(request, controller.signal, (chunk) => subscriber.next(chunk))
                .then(() => subscriber.complete())
                .catch((error: unknown) => {
                    // An abort is how this stream is closed on purpose; it is
                    // not something the widget should report as a failure.
                    if (controller.signal.aborted) {
                        subscriber.complete();

                        return;
                    }

                    subscriber.error(error);
                });

            return () => controller.abort();
        });
    }

    private async read(
        request: AssistantChatRequest,
        signal: AbortSignal,
        onChunk: (chunk: AssistantChatChunk) => void
    ): Promise<void> {
        const response = await this.coreWindowService.fetch(this.apiPathService.get(ASSISTANT_CHAT_PATH), {
            method: 'POST',
            signal,
            headers: {
                'Content-Type': 'application/json',
                [AuthTokenName.ACCESS]: this.accessToken()
            },
            body: JSON.stringify(request)
        });

        if (!response.ok || !response.body) {
            throw new Error(`assistant request failed with status ${response.status}`);
        }

        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';

        for (;;) {
            const { done, value } = await reader.read();

            if (done) {
                break;
            }

            buffer += decoder.decode(value, { stream: true });

            // The gateway writes one JSON envelope per line, but a chunk of the
            // stream may end anywhere, so the tail is kept for the next read.
            const lines = buffer.split('\n');
            buffer = lines.pop() ?? '';

            for (const line of lines) {
                const chunk = this.parseLine(line);

                if (chunk) {
                    onChunk(chunk);
                }
            }
        }

        const tail = this.parseLine(buffer);

        if (tail) {
            onChunk(tail);
        }
    }

    private parseLine(line: string): AssistantChatChunk | undefined {
        const trimmed = line.trim();

        if (!trimmed) {
            return undefined;
        }

        let envelope: AssistantChatStreamEnvelope;

        try {
            envelope = JSON.parse(trimmed) as AssistantChatStreamEnvelope;
        } catch {
            // A line that isn't JSON is not something the widget can act on,
            // and dropping it keeps one malformed chunk from ending the answer.
            return undefined;
        }

        if (envelope.error) {
            throw new Error(envelope.error.message || 'assistant request failed');
        }

        return envelope.result;
    }

    private accessToken(): string {
        return this.coreWindowService.localStorage.getItem(AuthTokenName.ACCESS) || '';
    }
}
