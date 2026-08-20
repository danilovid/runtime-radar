import { createAction, props } from '@ngrx/store';

import {
    AssistantAttachment,
    AssistantConversation,
    AssistantMessage,
    AssistantStopReason,
    AssistantToolActivityResponse,
    AssistantView
} from '../interfaces';

export const OPEN_ASSISTANT_TODO_ACTION = createAction(
    '[Assistant] Open',
    props<{ eventId?: string; question?: string }>()
);

export const CLOSE_ASSISTANT_TODO_ACTION = createAction('[Assistant] Close');

export const SHOW_ASSISTANT_VIEW_TODO_ACTION = createAction('[Assistant] Show View', props<{ view: AssistantView }>());

/** Starts a conversation, optionally asking its first question straight away. */
export const START_ASSISTANT_CHAT_TODO_ACTION = createAction(
    '[Assistant] Start Chat',
    props<{ question?: string; eventId?: string }>()
);

export const OPEN_ASSISTANT_CONVERSATION_TODO_ACTION = createAction(
    '[Assistant] Open Conversation',
    props<{ conversationId: string }>()
);

export const DELETE_ASSISTANT_CONVERSATION_TODO_ACTION = createAction(
    '[Assistant] Delete Conversation',
    props<{ conversationId: string }>()
);

export const SELECT_ASSISTANT_INTEGRATION_TODO_ACTION = createAction(
    '[Assistant] Select Integration',
    props<{ integrationId: string }>()
);

export const SEND_ASSISTANT_MESSAGE_TODO_ACTION = createAction(
    '[Assistant] Send Message',
    props<{ content: string }>()
);

export const ATTACH_ASSISTANT_FILES_TODO_ACTION = createAction('[Assistant] Attach Files', props<{ files: File[] }>());

export const REMOVE_ASSISTANT_ATTACHMENT_TODO_ACTION = createAction(
    '[Assistant] Remove Attachment',
    props<{ name: string }>()
);

export const SET_ASSISTANT_OPEN_DOC_ACTION = createAction('[Assistant] (Doc) Set Open', props<{ isOpen: boolean }>());

export const SET_ASSISTANT_VIEW_DOC_ACTION = createAction(
    '[Assistant] (Doc) Set View',
    props<{ view: AssistantView }>()
);

export const SET_ASSISTANT_INTEGRATION_DOC_ACTION = createAction(
    '[Assistant] (Doc) Set Integration',
    props<{ integrationId: string }>()
);

export const ADD_ASSISTANT_CONVERSATION_DOC_ACTION = createAction(
    '[Assistant] (Doc) Add Conversation',
    props<{ conversation: AssistantConversation }>()
);

export const SET_ASSISTANT_ACTIVE_CONVERSATION_DOC_ACTION = createAction(
    '[Assistant] (Doc) Set Active Conversation',
    props<{ conversationId: string }>()
);

export const DELETE_ASSISTANT_CONVERSATION_DOC_ACTION = createAction(
    '[Assistant] (Doc) Delete Conversation',
    props<{ conversationId: string }>()
);

export const ADD_ASSISTANT_MESSAGE_DOC_ACTION = createAction(
    '[Assistant] (Doc) Add Message',
    props<{ message: AssistantMessage }>()
);

/** A piece of the answer, appended to the message being streamed. */
export const APPEND_ASSISTANT_DELTA_DOC_ACTION = createAction(
    '[Assistant] (Doc) Append Delta',
    props<{ delta: string }>()
);

export const UPDATE_ASSISTANT_TOOL_DOC_ACTION = createAction(
    '[Assistant] (Doc) Update Tool',
    props<{ activity: AssistantToolActivityResponse }>()
);

export const FINISH_ASSISTANT_MESSAGE_DOC_ACTION = createAction(
    '[Assistant] (Doc) Finish Message',
    props<{ stopReason?: AssistantStopReason; error?: string }>()
);

export const SET_ASSISTANT_ATTACHMENTS_DOC_ACTION = createAction(
    '[Assistant] (Doc) Set Attachments',
    props<{ attachments: AssistantAttachment[] }>()
);
