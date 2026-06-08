create table if not exists neutron_project(
    id char(36) primary key,
    webhook_type varchar(20),
    repo_url varchar(200)
);
create table if not exists neutron_job(
    id bigint primary key auto_increment,
    project_id char(36) not null,
    name varchar(255) not null,
    status text not null,
    completed bool not null default false,
    completed_at timestamp null,
    unique index idx_name (name)
);
create table if not exists neutron_pod(
    id bigint primary key auto_increment,
    job_id bigint not null,
    pod_name varchar(255),
    pod_uid varchar(255),
    phase varchar(50),
    index idx_job_id (job_id)
);
