import { Injectable } from '@angular/core';
import { Action, Store } from '@ngrx/store';
import { Actions, createEffect, ofType } from '@ngrx/effects';
import { EMPTY, Observable, concat, from, of } from 'rxjs';
import { KbqToastService, KbqToastStyle } from '@koobiq/components/toast';
import { catchError, filter, map, mergeMap, switchMap, withLatestFrom } from 'rxjs/operators';

import { I18nService } from '@cs/i18n';

import { ASSISTANT_MAX_ATTACHMENTS } from '../constants/assistant.constant';
import { AssistantAttachmentService } from '../services/assistant-attachment.service';
import { AssistantRequestService } from '../services/assistant-request.service';
import {
    ADD_ASSISTANT_CONVERSATION_DOC_ACTION,
    ADD_ASSISTANT_MESSAGE_DOC_ACTION,
    ADD_ASSISTANT_SECRET_DOC_ACTION,
    APPEND_ASSISTANT_DELTA_DOC_ACTION,
    ATTACH_ASSISTANT_FILES_TODO_ACTION,
    CLOSE_ASSISTANT_TODO_ACTION,
    CONFIRM_ASSISTANT_ACTION_TODO_ACTION,
    DECLINE_ASSISTANT_ACTION_TODO_ACTION,
    DELETE_ASSISTANT_CONVERSATION_DOC_ACTION,
    DELETE_ASSISTANT_CONVERSATION_TODO_ACTION,
    FINISH_ASSISTANT_MESSAGE_DOC_ACTION,
    LOAD_ASSISTANT_CHATS_TODO_ACTION,
    OPEN_ASSISTANT_CONVERSATION_TODO_ACTION,
    OPEN_ASSISTANT_TODO_ACTION,
    REMOVE_ASSISTANT_ATTACHMENT_TODO_ACTION,
    SELECT_ASSISTANT_INTEGRATION_TODO_ACTION,
    SEND_ASSISTANT_MESSAGE_TODO_ACTION,
    SET_ASSISTANT_ACTION_DOC_ACTION,
    SET_ASSISTANT_ACTION_STATE_DOC_ACTION,
    SET_ASSISTANT_ACTIVE_CONVERSATION_DOC_ACTION,
    SET_ASSISTANT_ATTACHMENTS_DOC_ACTION,
    SET_ASSISTANT_CHATS_DOC_ACTION,
    SET_ASSISTANT_INTEGRATION_DOC_ACTION,
    SET_ASSISTANT_OPEN_DOC_ACTION,
    SET_ASSISTANT_SUGGESTIONS_DOC_ACTION,
    SET_ASSISTANT_VIEW_DOC_ACTION,
    SHOW_ASSISTANT_VIEW_TODO_ACTION,
    START_ASSISTANT_CHAT_TODO_ACTION,
    UPDATE_ASSISTANT_TOOL_DOC_ACTION
} from './assistant-action.store';
import {
    AssistantActionState,
    AssistantAttachment,
    AssistantChatChunk,
    AssistantChatMessageRequest,
    AssistantChatRequest,
    AssistantConversation,
    AssistantEventKind,
    AssistantMessage,
    AssistantMode,
    AssistantRole,
    AssistantState,
    AssistantStopReason,
    AssistantStoredChat
} from '../interfaces';
import {
    getAssistantActiveConversation,
    getAssistantConversations,
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
            map(({ question, eventId, eventKind, mode }) =>
                START_ASSISTANT_CHAT_TODO_ACTION({ question, eventId, eventKind, mode })
            )
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
            mergeMap(({ question, eventId, eventKind, mode }) => {
                const now = new Date().toISOString();
                const conversation: AssistantConversation = {
                    id: this.identifier(),
                    title: '',
                    createdAt: now,
                    updatedAt: now,
                    messages: [],
                    eventId: eventId ?? '',
                    eventKind: eventKind ?? AssistantEventKind.RUNTIME,
                    chatId: '',
                    mode: mode ?? AssistantMode.CHAT
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

    // A conversation is deleted where it is kept. It leaves the screen either
    // way: a chat the server could not drop is still one the user asked to be
    // rid of, and it will come back on the next load rather than be lost.
    readonly deleteConversation$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(DELETE_ASSISTANT_CONVERSATION_TODO_ACTION),
            withLatestFrom(this.store.select(getAssistantConversations)),
            mergeMap(([{ conversationId }, conversations]) => {
                const chatId = conversations.find((item) => item.id === conversationId)?.chatId;
                const removed = DELETE_ASSISTANT_CONVERSATION_DOC_ACTION({ conversationId });

                if (!chatId) {
                    return of(removed);
                }

                return this.assistantRequestService.deleteChat(chatId).pipe(
                    map(() => removed),
                    catchError(() => of(removed))
                );
            })
        )
    );

    /** Reads back what the server kept, so a reload does not lose the chats. */
    readonly loadChats$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(LOAD_ASSISTANT_CHATS_TODO_ACTION),
            switchMap(() =>
                this.assistantRequestService.listChats().pipe(
                    map((response) =>
                        SET_ASSISTANT_CHATS_DOC_ACTION({
                            conversations: (response.chats ?? []).map((chat) => storedChat(chat))
                        })
                    ),
                    // A listing that fails leaves the tab with what it has;
                    // there is nothing useful to tell the user about it.
                    catchError(() => of(SET_ASSISTANT_CHATS_DOC_ACTION({ conversations: [] })))
                )
            )
        )
    );

    /** Loads the turns of a stored conversation the first time it is opened. */
    readonly readChat$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(OPEN_ASSISTANT_CONVERSATION_TODO_ACTION),
            withLatestFrom(this.store.select(getAssistantConversations)),
            mergeMap(([{ conversationId }, conversations]) => {
                const conversation = conversations.find((item) => item.id === conversationId);

                if (!conversation?.chatId || conversation.messages.length) {
                    return EMPTY;
                }

                return this.assistantRequestService.readChat(conversation.chatId).pipe(
                    map((response) =>
                        SET_ASSISTANT_CHATS_DOC_ACTION({
                            conversations: conversations.map((item) =>
                                item.id === conversationId && response.chat ? storedChat(response.chat) : item
                            )
                        })
                    ),
                    catchError(() => EMPTY)
                );
            })
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
            switchMap(([{ files }, pending]) => {
                // How many more files this message may carry is decided before
                // anything is read, so that the shared byte budget is not spent
                // on a file that would be dropped for being the fourth one.
                const room = Math.max(ASSISTANT_MAX_ATTACHMENTS - pending.length, 0);

                return from(this.assistantAttachmentService.readWithinBudget(files.slice(0, room), pending)).pipe(
                    map((attachments) =>
                        SET_ASSISTANT_ATTACHMENTS_DOC_ACTION({
                            attachments: [...pending, ...attachments]
                        })
                    ),
                    catchError(() => {
                        this.toastService.show({
                            style: KbqToastStyle.Warning,
                            title: this.i18nService.translate('Assistant.Pseudo.Notification.AttachmentFailed')
                        });

                        return of(SET_ASSISTANT_ATTACHMENTS_DOC_ACTION({ attachments: pending }));
                    })
                );
            })
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

                return concat(
                    of(
                        SET_ASSISTANT_ATTACHMENTS_DOC_ACTION({ attachments: [] }),
                        ADD_ASSISTANT_MESSAGE_DOC_ACTION({ message: question }),
                        ADD_ASSISTANT_MESSAGE_DOC_ACTION({ message: answer })
                    ),
                    this.stream({
                        integration_id: integrationId,
                        conversation: this.payload([...history, question]),
                        ...(conversation?.eventId
                            ? { event_id: conversation.eventId, event_kind: conversation.eventKind }
                            : {}),
                        ...(conversation?.chatId ? { chat_id: conversation.chatId } : {}),
                        ...(conversation?.mode ? { mode: conversation.mode } : {})
                    })
                );
            })
        )
    );

    /**
     * Runs the change the assistant proposed, now that the user approved it.
     * The request carries only the identifier: the arguments live on the server
     * exactly as they were shown, so nothing between the two can alter them.
     */
    readonly confirmAction$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(CONFIRM_ASSISTANT_ACTION_TODO_ACTION),
            withLatestFrom(
                this.store.select(getAssistantActiveConversation),
                this.store.select(getAssistantIntegrationId)
            ),
            filter(([, conversation, integrationId]) => !!conversation && !!integrationId),
            switchMap(([{ messageId, actionId }, conversation, integrationId]) => {
                const answer = this.message(AssistantRole.ASSISTANT, '', []);
                answer.isPending = true;

                return concat(
                    of(
                        SET_ASSISTANT_ACTION_STATE_DOC_ACTION({ messageId, state: AssistantActionState.APPROVED }),
                        ADD_ASSISTANT_MESSAGE_DOC_ACTION({ message: answer })
                    ),
                    this.stream({
                        integration_id: integrationId,
                        conversation: this.payload(conversation?.messages ?? []),
                        confirm_id: actionId,
                        ...(conversation?.eventId
                            ? { event_id: conversation.eventId, event_kind: conversation.eventKind }
                            : {}),
                        ...(conversation?.chatId ? { chat_id: conversation.chatId } : {}),
                        ...(conversation?.mode ? { mode: conversation.mode } : {})
                    })
                );
            })
        )
    );

    /**
     * Declining costs nothing to record: the proposal is marked as refused and
     * the identifier is simply never used, so it expires on the server.
     */
    readonly declineAction$: Observable<Action> = createEffect(() =>
        this.actions$.pipe(
            ofType(DECLINE_ASSISTANT_ACTION_TODO_ACTION),
            map(({ messageId }) =>
                SET_ASSISTANT_ACTION_STATE_DOC_ACTION({ messageId, state: AssistantActionState.DECLINED })
            )
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

    /**
     * Streams one answer into the store. Both asking a question and approving
     * an action end here: the only difference is what the request carries.
     */
    private stream(request: AssistantChatRequest): Observable<Action> {
        return concat(
            this.assistantRequestService.chat(request).pipe(
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
            // dropped — must still take the answer out of its pending state.
            // Finishing an already finished message is a no-op.
            of(FINISH_ASSISTANT_MESSAGE_DOC_ACTION({}))
        );
    }

    /**
     * Renders the history for the server. Only what the user and the assistant
     * said is replayed: tool traffic belongs to the server side of the loop,
     * and a failed or empty turn would only confuse the model.
     */
    private payload(messages: AssistantMessage[]): AssistantChatMessageRequest[] {
        return messages
            .filter((message) => !message.error && !!message.content.trim())
            .map((message) => ({
                role: message.role,
                content: message.content + this.assistantAttachmentService.render(message.attachments)
            }));
    }

    private message(role: AssistantRole, content: string, attachments: AssistantAttachment[]): AssistantMessage {
        return {
            id: this.identifier(),
            role,
            content,
            tools: [],
            attachments,
            secrets: [],
            isPending: false
        };
    }

    private toAction(chunk: AssistantChatChunk): Action {
        if (chunk.tool_activity) {
            return UPDATE_ASSISTANT_TOOL_DOC_ACTION({ activity: chunk.tool_activity });
        }

        if (chunk.confirmation) {
            return SET_ASSISTANT_ACTION_DOC_ACTION({
                action: {
                    id: chunk.confirmation.id,
                    tool: chunk.confirmation.tool,
                    title: chunk.confirmation.title,
                    arguments: chunk.confirmation.arguments,
                    isDestructive: !!chunk.confirmation.destructive,
                    state: AssistantActionState.PENDING
                }
            });
        }

        if (chunk.secret) {
            return ADD_ASSISTANT_SECRET_DOC_ACTION({ secret: chunk.secret });
        }

        if (chunk.suggestions) {
            return SET_ASSISTANT_SUGGESTIONS_DOC_ACTION({ suggestions: chunk.suggestions.questions ?? [] });
        }

        if (chunk.done) {
            return FINISH_ASSISTANT_MESSAGE_DOC_ACTION({
                stopReason: chunk.done.stop_reason as AssistantStopReason,
                chatId: chunk.done.chat_id,
                error: chunk.done.error
            });
        }

        return APPEND_ASSISTANT_DELTA_DOC_ACTION({ delta: chunk.delta ?? '' });
    }

    private identifier(): string {
        return `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
    }
}

/**
 * A stored conversation as the widget holds it. The server's identifier is kept
 * in both fields: the list is keyed by id, and chatId is what the next turn and
 * a delete are addressed to.
 */
function storedChat(chat: AssistantStoredChat): AssistantConversation {
    return {
        id: chat.id,
        chatId: chat.id,
        title: chat.title,
        createdAt: chat.created_at,
        updatedAt: chat.updated_at,
        eventId: chat.event_id ?? '',
        eventKind: (chat.event_kind as AssistantEventKind) || AssistantEventKind.RUNTIME,
        mode: (chat.mode as AssistantMode) || AssistantMode.CHAT,
        messages: (chat.messages ?? []).map((message, index) => ({
            id: `${chat.id}-${index}`,
            role: message.role as AssistantRole,
            content: message.content,
            tools: [],
            attachments: [],
            secrets: [],
            isPending: false
        }))
    };
}
