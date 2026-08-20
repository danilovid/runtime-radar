import { Injectable } from '@angular/core';
import { Action, Store } from '@ngrx/store';
import { Actions, createEffect, ofType } from '@ngrx/effects';
import { KbqToastService, KbqToastStyle } from '@koobiq/components/toast';
import { Observable, concat, from, of } from 'rxjs';
import { catchError, filter, map, mergeMap, switchMap, withLatestFrom } from 'rxjs/operators';

import { I18nService } from '@cs/i18n';

import { ASSISTANT_MAX_ATTACHMENTS } from '../constants/assistant.constant';
import { AssistantAttachmentService } from '../services/assistant-attachment.service';
import { AssistantRequestService } from '../services/assistant-request.service';
import {
    ADD_ASSISTANT_CONVERSATION_DOC_ACTION,
    ADD_ASSISTANT_MESSAGE_DOC_ACTION,
    APPEND_ASSISTANT_DELTA_DOC_ACTION,
    ATTACH_ASSISTANT_FILES_TODO_ACTION,
    CLOSE_ASSISTANT_TODO_ACTION,
    DELETE_ASSISTANT_CONVERSATION_DOC_ACTION,
    DELETE_ASSISTANT_CONVERSATION_TODO_ACTION,
    FINISH_ASSISTANT_MESSAGE_DOC_ACTION,
    OPEN_ASSISTANT_CONVERSATION_TODO_ACTION,
    OPEN_ASSISTANT_TODO_ACTION,
    REMOVE_ASSISTANT_ATTACHMENT_TODO_ACTION,
    SELECT_ASSISTANT_INTEGRATION_TODO_ACTION,
    SEND_ASSISTANT_MESSAGE_TODO_ACTION,
    SET_ASSISTANT_ACTIVE_CONVERSATION_DOC_ACTION,
    SET_ASSISTANT_ATTACHMENTS_DOC_ACTION,
    SET_ASSISTANT_INTEGRATION_DOC_ACTION,
    SET_ASSISTANT_OPEN_DOC_ACTION,
    SET_ASSISTANT_VIEW_DOC_ACTION,
    SHOW_ASSISTANT_VIEW_TODO_ACTION,
    START_ASSISTANT_CHAT_TODO_ACTION,
    UPDATE_ASSISTANT_TOOL_DOC_ACTION
} from './assistant-action.store';
import {
    AssistantAttachment,
    AssistantChatChunk,
    AssistantChatMessageRequest,
    AssistantConversation,
    AssistantMessage,
    AssistantRole,
    AssistantState,
    AssistantStopReason
} from '../interfaces';
import {
    getAssistantActiveConversation,
    getAssistantIntegrationId,
    getAssistantPendingAttachments
} from './assistant-selector.store';

@Injectable({
    providedIn: 'root'
})
export class AssistantEffectStore {
    readonly open$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(OPEN_ASSISTANT_TODO_ACTION),
            map(() => SET_ASSISTANT_OPEN_DOC_ACTION({ isOpen: true }))
        )
    );

    /**
     * Opening with a question or an event goes straight into a conversation —
     * that is what "Ask the assistant" on an event page does. Opening without
     * either lands on the home screen.
     */
    readonly openWithQuestion$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(OPEN_ASSISTANT_TODO_ACTION),
            filter(({ question, eventId }) => !!question || !!eventId),
            map(({ question, eventId }) => START_ASSISTANT_CHAT_TODO_ACTION({ question, eventId }))
        )
    );

    readonly close$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(CLOSE_ASSISTANT_TODO_ACTION),
            map(() => SET_ASSISTANT_OPEN_DOC_ACTION({ isOpen: false }))
        )
    );

    readonly showView$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(SHOW_ASSISTANT_VIEW_TODO_ACTION),
            map(({ view }) => SET_ASSISTANT_VIEW_DOC_ACTION({ view }))
        )
    );

    readonly startChat$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(START_ASSISTANT_CHAT_TODO_ACTION),
            mergeMap(({ question, eventId }) => {
                const now = new Date().toISOString();
                const conversation: AssistantConversation = {
                    id: this.identifier(),
                    title: '',
                    createdAt: now,
                    updatedAt: now,
                    messages: [],
                    eventId: eventId ?? ''
                };

                const started: Action[] = [ADD_ASSISTANT_CONVERSATION_DOC_ACTION({ conversation })];

                return question ? [...started, SEND_ASSISTANT_MESSAGE_TODO_ACTION({ content: question })] : started;
            })
        )
    );

    readonly openConversation$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(OPEN_ASSISTANT_CONVERSATION_TODO_ACTION),
            mergeMap(({ conversationId }) => [
                SET_ASSISTANT_OPEN_DOC_ACTION({ isOpen: true }),
                SET_ASSISTANT_ACTIVE_CONVERSATION_DOC_ACTION({ conversationId })
            ])
        )
    );

    readonly deleteConversation$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(DELETE_ASSISTANT_CONVERSATION_TODO_ACTION),
            map(({ conversationId }) => DELETE_ASSISTANT_CONVERSATION_DOC_ACTION({ conversationId }))
        )
    );

    readonly selectIntegration$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(SELECT_ASSISTANT_INTEGRATION_TODO_ACTION),
            map(({ integrationId }) => SET_ASSISTANT_INTEGRATION_DOC_ACTION({ integrationId }))
        )
    );

    /** Reads picked files in the browser; nothing is uploaded anywhere. */
    readonly attachFiles$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(ATTACH_ASSISTANT_FILES_TODO_ACTION),
            withLatestFrom(this.store.select(getAssistantPendingAttachments)),
            switchMap(([{ files }, pending]) =>
                from(Promise.all(files.map((file) => this.assistantAttachmentService.read(file)))).pipe(
                    map((attachments) =>
                        SET_ASSISTANT_ATTACHMENTS_DOC_ACTION({
                            attachments: [...pending, ...attachments].slice(0, ASSISTANT_MAX_ATTACHMENTS)
                        })
                    ),
                    catchError(() => {
                        this.toastService.show({
                            style: KbqToastStyle.Warning,
                            title: this.i18nService.translate('Assistant.Pseudo.Notification.AttachmentFailed')
                        });

                        return of(SET_ASSISTANT_ATTACHMENTS_DOC_ACTION({ attachments: pending }));
                    })
                )
            )
        )
    );

    readonly removeAttachment$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(REMOVE_ASSISTANT_ATTACHMENT_TODO_ACTION),
            withLatestFrom(this.store.select(getAssistantPendingAttachments)),
            map(([{ name }, pending]) =>
                SET_ASSISTANT_ATTACHMENTS_DOC_ACTION({
                    attachments: pending.filter((attachment) => attachment.name !== name)
                })
            )
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
                this.store.select(getAssistantActiveConversation),
                this.store.select(getAssistantIntegrationId),
                this.store.select(getAssistantPendingAttachments)
            ),
            filter(
                ([{ content }, conversation, integrationId]) => !!content.trim() && !!conversation && !!integrationId
            ),
            switchMap(([{ content }, conversation, integrationId, attachments]) => {
                const question = this.message(AssistantRole.USER, content.trim(), attachments);
                const answer = this.message(AssistantRole.ASSISTANT, '', []);
                answer.isPending = true;

                const history = conversation?.messages ?? [];
                const payload: AssistantChatMessageRequest[] = [...history, question]
                    .filter((message) => !message.error)
                    .map((message) => ({
                        role: message.role,
                        content: message.content + this.assistantAttachmentService.render(message.attachments)
                    }));

                return concat(
                    of(
                        SET_ASSISTANT_ATTACHMENTS_DOC_ACTION({ attachments: [] }),
                        ADD_ASSISTANT_MESSAGE_DOC_ACTION({ message: question }),
                        ADD_ASSISTANT_MESSAGE_DOC_ACTION({ message: answer })
                    ),
                    this.assistantRequestService
                        .chat({
                            integration_id: integrationId,
                            conversation: payload,
                            ...(conversation?.eventId ? { event_id: conversation.eventId } : {})
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
        private readonly assistantAttachmentService: AssistantAttachmentService,
        private readonly assistantRequestService: AssistantRequestService,
        private readonly i18nService: I18nService,
        private readonly store: Store<AssistantState>,
        private readonly toastService: KbqToastService
    ) {}

    private message(role: AssistantRole, content: string, attachments: AssistantAttachment[]): AssistantMessage {
        return {
            id: this.identifier(),
            role,
            content,
            tools: [],
            attachments,
            isPending: false
        };
    }

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

    private identifier(): string {
        return `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
    }
}
