import { AssistantAttachment, AssistantConversation, AssistantView } from '../assistant-message.interface';

export interface AssistantState {
    /**
     * Whether the chat panel is expanded. The widget itself is always on
     * screen: collapsed it is a one-line composer, and typing into it opens
     * the panel with the question already asked.
     */
    isOpen: boolean;
    /** Which screen the widget shows. */
    view: AssistantView;
    /** The AI integration answering. Empty until one is picked or defaulted. */
    integrationId: string;
    /**
     * Every conversation of this session, newest first. They live for as long
     * as the page does — the chats page reads them from here.
     *
     * Extension point: this is what a Postgres-backed history would replace,
     * with the effect store loading and saving instead of keeping them here.
     */
    conversations: AssistantConversation[];
    /** The conversation the widget shows. Empty on the home screen. */
    activeConversationId: string;
    /** Whether an answer is being streamed right now. */
    isStreaming: boolean;
    /** Files picked in the composer but not sent yet. */
    pendingAttachments: AssistantAttachment[];
}
