package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ ephemeral.EphemeralResource          = &AuthEphemeralResource{}
	_ ephemeral.EphemeralResourceWithClose = &AuthEphemeralResource{}
)

func newAuthEphemeralResource() ephemeral.EphemeralResource {
	return &AuthEphemeralResource{}
}

type AuthEphemeralResource struct {
	client *CephAPIClient
}

type AuthEphemeralResourceModel struct {
	Entity  types.String `tfsdk:"entity"`
	Caps    types.Map    `tfsdk:"caps"`
	Key     types.String `tfsdk:"key"`
	Keyring types.String `tfsdk:"keyring"`
}

func (r *AuthEphemeralResource) Metadata(ctx context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_auth_ephemeral"
}

func (r *AuthEphemeralResource) Schema(ctx context.Context, req ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "This ephemeral resource allows you to manage a ceph client authentication.",
		Attributes: map[string]schema.Attribute{
			"entity": schema.StringAttribute{
				MarkdownDescription: "The entity name (i.e.: client.admin)",
				Required:            true,
			},
			"caps": schema.MapAttribute{
				ElementType:         types.StringType,
				MarkdownDescription: "The caps of the entity",
				Required:            true,
			},
			"key": schema.StringAttribute{
				MarkdownDescription: "The generated cephx key of the entity.",
				Computed:            true,
				Sensitive:           true,
			},
			"keyring": schema.StringAttribute{
				MarkdownDescription: "The complete cephx keyring as JSON",
				Computed:            true,
				Sensitive:           true,
			},
		},
	}
}

func (r *AuthEphemeralResource) Configure(ctx context.Context, req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*CephAPIClient)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *CephAPIClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *AuthEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	var data AuthEphemeralResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	entity := data.Entity.ValueString()

	caps, ok := mapAttrToCephCaps(ctx, data.Caps, &resp.Diagnostics)
	if !ok {
		return
	}

	err := r.client.ClusterCreateUser(ctx, entity, caps)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to create user in Ceph API: %s", err),
		)
		return
	}

	entityJSON, err := json.Marshal(entity)
	if err != nil {
		resp.Diagnostics.AddError(
			"Private State Error",
			fmt.Sprintf("Unable to marshal entity to JSON: %s", err),
		)
		return
	}
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, "entity", entityJSON)...)
	if resp.Diagnostics.HasError() {
		return
	}

	updateAuthEphemeralModelFromCephExport(ctx, r.client, entity, &data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.Result.Set(ctx, &data)...)
}

func (r *AuthEphemeralResource) Close(ctx context.Context, req ephemeral.CloseRequest, resp *ephemeral.CloseResponse) {
	entityBytes, diags := req.Private.GetKey(ctx, "entity")
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var entity string
	if err := json.Unmarshal(entityBytes, &entity); err != nil {
		resp.Diagnostics.AddError(
			"Private State Error",
			fmt.Sprintf("Unable to unmarshal entity from JSON: %s", err),
		)
		return
	}

	err := r.client.ClusterDeleteUser(ctx, entity)
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to delete user from Ceph API: %s", err),
		)
		return
	}
}

func updateAuthEphemeralModelFromCephExport(ctx context.Context, client *CephAPIClient, entity string, data *AuthEphemeralResourceModel, diagnostics *diag.Diagnostics) {
	resourceModel := AuthResourceModel{
		Entity:  data.Entity,
		Caps:    data.Caps,
		Key:     data.Key,
		Keyring: data.Keyring,
	}

	updateAuthModelFromCephExport(ctx, client, entity, &resourceModel, diagnostics)
	if diagnostics.HasError() {
		return
	}

	data.Caps = resourceModel.Caps
	data.Key = resourceModel.Key
	data.Keyring = resourceModel.Keyring
}
