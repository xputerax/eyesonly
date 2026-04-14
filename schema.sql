CREATE TABLE secrets (
    id integer primary key,
    edit_id varchar(255) not null,
    peek_id varchar(255) not null,
    title varchar(255) not null,
    content varchar(255) not null,
    password varchar(255),
    expire_at datetime
);