CREATE TABLE IF NOT EXISTS notices (
    id text PRIMARY KEY,
    heading text NOT NULL,
    type text NOT NULL,
    status text,
    issue_date timestamptz NOT NULL,
    deadline timestamptz,
    estimated_amount numeric(15,2),
    currency text
);

CREATE TABLE IF NOT EXISTS buyers (
    organization_id text PRIMARY KEY,
    name text NOT NULL
);

CREATE TABLE IF NOT EXISTS notice_buyers (
    notice_id text NOT NULL REFERENCES notices(id) ON DELETE CASCADE,
    organization_id text NOT NULL REFERENCES buyers(organization_id) ON DELETE CASCADE,
    PRIMARY KEY (notice_id, organization_id)
);

CREATE TABLE IF NOT EXISTS notice_cpv_codes (
    notice_id text NOT NULL REFERENCES notices(id) ON DELETE CASCADE,
    cpv_code text NOT NULL,
    PRIMARY KEY (notice_id, cpv_code)
);

CREATE INDEX IF NOT EXISTS notice_buyers_organization_id_idx ON notice_buyers(organization_id);
CREATE INDEX IF NOT EXISTS notice_cpv_codes_cpv_code_idx
    ON notice_cpv_codes (cpv_code text_pattern_ops);