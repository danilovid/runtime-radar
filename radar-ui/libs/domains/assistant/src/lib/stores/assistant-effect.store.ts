import { Injectable } from '@angular/core';
import { Action, Store } from '@ngrx/store';
import { Actions, createEffect, ofType } from '@ngrx/effects';
import { KbqToastService, KbqToastStyle } from '@koobiq/components/toast';
import { Observable, concat, of } from 'rxjs';
import { catchError, filter, map, switchMap, withLatestFrom } from 'rxjs/operators';

import { I18nService } from '@cs/i18n';

import { AssistantRequestService } from '../services/assistant-request.service';
import {
    ADD_ASSISTANT_MESSAGE_DOC_ACTION,
    APPEND_ASSISTANT_DELTA_DOC_ACTION,
    CLEAR_ASSISTANT_CONVERSATION_DOC_ACTION,
    CLEAR_ASSISTANT_CONVERSATION_TODO_ACTION,
    CLOSE_ASSISTANT_TODO_ACTION,
    FINISH_ASSISTANT_MESSAGE_DOC_ACTION,
    OPEN_ASSISTANT_TODO_ACTION,
    SELECT_ASSISTANT_INTEGRATION_TODO_ACTION,
    SEND_ASSISTANT_MESSAGE_TODO_ACTION,
    SET_ASSISTANT_INTEGRATION_DOC_ACTION,
    SET_ASSISTANT_OPEN_DOC_ACTION,
    UPDATE_ASSISTANT_TOOL_DOC_ACTION
} from './assistant-action.store';
import {
    AssistantChatChunk,
    AssistantChatMessageRequest,
    AssistantMessage,
    AssistantRole,
    AssistantState,
    AssistantStopReason
} from '../interfaces';
import { getAssistantEventId, getAssistantIntegrationId, getAssistantMessages } from './assistant-selector.store';

@Injectable({
    providedIn: 'root'
})
export class AssistantEffectStore {
    readonly open$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(OPEN_ASSISTANT_TODO_ACTION),
            map(({ eventId }) => SET_ASSISTANT_OPEN_DOC_ACTION({ isOpen: true, eventId }))
        )
    );

    /**
     * Opening the widget with a question asks it straight away, which is what
     * the "Ask the assistant" action on an event page does.
     */
    readonly openWithQuestion$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(OPEN_ASSISTANT_TODO_ACTION),
            filter(({ question }) => !!question),
            map(({ question }) => SEND_ASSISTANT_MESSAGE_TODO_ACTION({ content: question as string }))
        )
    );

    readonly close$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(CLOSE_ASSISTANT_TODO_ACTION),
            map(() => SET_ASSISTANT_OPEN_DOC_ACTION({ isOpen: false }))
        )
    );

    readonly clear$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(CLEAR_ASSISTANT_CONVERSATION_TODO_ACTION),
            map(() => CLEAR_ASSISTANT_CONVERSATION_DOC_ACTION())
        )
    );

    readonly selectIntegration$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(SELECT_ASSISTANT_INTEGRATION_TODO_ACTION),
            map(({ integrationId }) => SET_ASSISTANT_INTEGRATION_DOC_ACTION({ integrationId }))
        )
    );

    /**
     * Sends a question and turns the answer's chunks into store updates as they
     * arrive. switchMap is deliberate: asking again abandons the previous
     * answer, and unsubscribing aborts its request.
     */
    readonly send$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(SEND_ASSISTANT_MESSAGE_TODO_ACTION),
            withLatestFrom(
                this.store.select(getAssistantMessages),
                this.store.select(getAssistantIntegrationId),
                this.store.select(getAssistantEventId)
            ),
            filter(([{ content }, , integrationId]) => !!content.trim() && !!integrationId),
            switchMap(([{ content }, messages, integrationId, eventId]) => {
                const question: AssistantMessage = {
                    id: this.messageId(),
                    role: AssistantRole.USER,
                    content: content.trim(),
                    tools: [],
                    isPending: false
                };

                const answer: AssistantMessage = {
                    id: this.messageId(),
                    role: AssistantRole.ASSISTANT,
                    content: '',
                    tools: [],
                    isPending: true
                };

                const conversation: AssistantChatMessageRequest[] = [...messages, question]
                    .filter((message) => !message.error)
                    .map((message) => ({ role: message.role, content: message.content }));

                return concat(
                    of(
                        ADD_ASSISTANT_MESSAGE_DOC_ACTION({ message: question }),
                        ADD_ASSISTANT_MESSAGE_DOC_ACTION({ message: answer })
                    ),
                    this.assistantRequestService
                        .chat({
                            integration_id: integrationId,
                            conversation,
                            ...(eventId ? { event_id: eventId } : {})
                        })
                        .pipe(
                            map((chunk) => this.toAction(chunk)),
                            catchError((error: Error) => {
                                this.toastService.show({
                                    style: KbqToastStyle.Warning,
                                    title: this.i18nService.translate('Assistant.Pseudo.Notification.ChatFailed')
                                });

                                return of(FINISH_ASSISTANT_MESSAGE_DOC_ACTION({ error: error.message }));
                            })
                        ),
                    // A stream that ended without a done chunk — the connection
                    // dropped — must still take the answer out of its pending
                    // state. Finishing an already finished message is a no-op.
                    of(FINISH_ASSISTANT_MESSAGE_DOC_ACTION({}))
                );
            })
        )
    );

    constructor(
        private readonly actions$: Actions,
        private readonly assistantRequestService: AssistantRequestService,
        private readonly i18nService: I18nService,
        private readonly store: Store<AssistantState>,
        private readonly toastService: KbqToastService
    ) {}

    private toAction(chunk: AssistantChatChunk): Action {
        if (chunk.tool_activity) {
            return UPDATE_ASSISTANT_TOOL_DOC_ACTION({ activity: chunk.tool_activity });
        }

        if (chunk.done) {
            return FINISH_ASSISTANT_MESSAGE_DOC_ACTION({
                stopReason: chunk.done.stop_reason as AssistantStopReason,
                error: chunk.done.error
            });
        }

        return APPEND_ASSISTANT_DELTA_DOC_ACTION({ delta: chunk.delta ?? '' });
    }

    private messageId(): string {
        return `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
    }
}
