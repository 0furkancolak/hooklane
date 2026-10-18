export type EventTemplate = { label: string; eventType: string; payload: unknown };

export const eventTemplates: EventTemplate[] = [
  { label: "Invoice paid", eventType: "invoice.paid", payload: { invoice_id: "inv_1042", amount: 4900, currency: "usd", customer: "cus_8a1f" } },
  { label: "User signed up", eventType: "user.created", payload: { user_id: "usr_2291", email: "ada@example.com", plan: "free" } },
  { label: "Order shipped", eventType: "order.shipped", payload: { order_id: "ord_7731", carrier: "UPS", tracking: "1Z999AA10123456784" } },
];

export function formatPayload(payload: unknown): string {
  return JSON.stringify(payload, null, 2);
}
