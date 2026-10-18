import type { MessageKey } from "../../shared/i18n/en";

export type EventTemplate = { label: MessageKey; eventType: string; payload: unknown };

export const eventTemplates: EventTemplate[] = [
  { label: "playground.template.invoice", eventType: "invoice.paid", payload: { invoice_id: "inv_1042", amount: 4900, currency: "usd", customer: "cus_8a1f" } },
  { label: "playground.template.user", eventType: "user.created", payload: { user_id: "usr_2291", email: "ada@example.com", plan: "free" } },
  { label: "playground.template.order", eventType: "order.shipped", payload: { order_id: "ord_7731", carrier: "UPS", tracking: "1Z999AA10123456784" } },
];

export function formatPayload(payload: unknown): string {
  return JSON.stringify(payload, null, 2);
}
