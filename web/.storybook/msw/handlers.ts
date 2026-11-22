import { http, HttpResponse } from "msw";

/** Synthetic responses for Storybook (docs/contracts/core-api.yaml). Fictional data only. */
export const storybookHandlers = [
  http.get("/me", () =>
    HttpResponse.json({
      user_id: "stu_demo_001",
      role: "student",
      display_name: "Демо-ученик",
    }),
  ),
  http.get("/assignments", () =>
    HttpResponse.json([
      {
        id: "asn_demo_001",
        kind: "homework",
        subject: "mathematics",
        state: "issued",
        deadline: "2026-12-01",
        answered: 3,
        total: 8,
        student_id: "stu_demo_001",
      },
      {
        id: "asn_demo_002",
        kind: "checkpoint",
        subject: "mathematics",
        state: "submitted",
        deadline: "2026-11-15",
        answered: 8,
        total: 8,
        student_id: "stu_demo_001",
      },
    ]),
  ),
];
