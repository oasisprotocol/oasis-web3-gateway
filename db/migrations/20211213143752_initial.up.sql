CREATE TABLE access_tuples (
    address character varying,
    storage_keys jsonb
);

--bun:split

CREATE TABLE blocks (
    hash character varying NOT NULL,
    round bigint,
    header jsonb
);

ALTER TABLE ONLY public.blocks
    ADD CONSTRAINT blocks_pkey PRIMARY KEY (hash);

CREATE UNIQUE INDEX block_on_round ON blocks USING btree (round);

--bun:split

CREATE TABLE logs (
    address character varying,
    topics jsonb,
    data character varying,
    round bigint,
    block_hash character varying,
    tx_hash character varying NOT NULL,
    tx_index bigint,
    index bigint NOT NULL,
    removed boolean
);

ALTER TABLE ONLY logs
    ADD CONSTRAINT logs_pkey PRIMARY KEY (tx_hash, index);

CREATE INDEX log_on_block_hash ON public.logs USING btree (block_hash);
CREATE INDEX log_on_round ON public.logs USING btree (round);

--bun:split

CREATE TABLE receipts (
    status bigint,
    cumulative_gas_used bigint,
    logs_bloom character varying,
    transaction_hash character varying NOT NULL,
    block_hash character varying,
    gas_used bigint,
    type bigint,
    round bigint,
    transaction_index bigint,
    from_addr character varying,
    to_addr character varying,
    contract_address character varying
);

ALTER TABLE ONLY receipts
    ADD CONSTRAINT receipts_pkey PRIMARY KEY (transaction_hash);

--bun:split

CREATE TABLE transactions (
    hash character varying NOT NULL,
    type smallint,
    status bigint,
    chain_id character varying,
    block_hash character varying,
    round bigint,
    index integer,
    gas bigint,
    gas_price character varying,
    gas_tip_cap character varying,
    gas_fee_cap character varying,
    nonce bigint,
    from_addr character varying,
    to_addr character varying,
    value character varying,
    data character varying,
    access_list jsonb,
    v character varying,
    r character varying,
    s character varying
);

ALTER TABLE ONLY transactions
    ADD CONSTRAINT transactions_pkey PRIMARY KEY (hash);

CREATE INDEX transaction_on_block_hash ON transactions USING btree (block_hash);
CREATE INDEX transaction_on_round ON transactions USING btree (round);

--bun:split

CREATE TABLE indexed_round_with_tips (
    tip character varying NOT NULL,
    round bigint
);
ALTER TABLE ONLY indexed_round_with_tips
    ADD CONSTRAINT indexed_round_with_tips_pkey PRIMARY KEY (tip);
