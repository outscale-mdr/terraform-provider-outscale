package outscale

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	oscgo "github.com/outscale/osc-sdk-go/v2"
)

func datasourceOutscaleOAPIServerCertificates() *schema.Resource {
	return &schema.Resource{
		Read: datasourceOutscaleOAPIServerCertificatesRead,
		Schema: map[string]*schema.Schema{
			"filter": dataSourceFiltersSchema(),
			"server_certificates": {
				Type:     schema.TypeList,
				Computed: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"expiration_date": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"id": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"name": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"path": {
							Type:     schema.TypeString,
							Computed: true,
						},
						"upload_date": {
							Type:     schema.TypeString,
							Computed: true,
						},
					},
				},
			},
			"request_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func datasourceOutscaleOAPIServerCertificatesRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*OutscaleClient).OSCAPI

	filters, filtersOk := d.GetOk("filter")

	// Build up search parameters
	params := oscgo.ReadServerCertificatesRequest{}

	if filtersOk {
		params.Filters = buildOutscaleOSCAPIDataSourceServerCertificateFilters(filters.(*schema.Set))
	}

	var resp oscgo.ReadServerCertificatesResponse
	var err error
	err = resource.Retry(120*time.Second, func() *resource.RetryError {
		resp, _, err = conn.ServerCertificateApi.ReadServerCertificates(context.Background()).ReadServerCertificatesRequest(params).Execute()

		if err != nil {
			if strings.Contains(err.Error(), "RequestLimitExceeded:") {
				return resource.RetryableError(err)
			}
			return resource.NonRetryableError(err)
		}
		return resource.RetryableError(err)
	})

	var errString string

	if err != nil {
		errString = err.Error()

		return fmt.Errorf("[DEBUG] Error reading Server Certificates (%s)", errString)
	}

	log.Printf("[DEBUG] Setting Server Certificates id (%s)", err)

	if err := d.Set("request_id", resp.ResponseContext.GetRequestId()); err != nil {
		return err
	}

	d.Set("server_certificates", flattenServerCertificates(resp.GetServerCertificates()))

	d.SetId(resource.UniqueId())

	return nil
}

func flattenServerCertificate(apiObject oscgo.ServerCertificate) map[string]interface{} {
	tfMap := map[string]interface{}{}
	tfMap["expiration_date"] = apiObject.GetExpirationDate()
	tfMap["id"] = apiObject.GetId()
	tfMap["name"] = apiObject.GetName()
	tfMap["path"] = apiObject.GetPath()
	tfMap["upload_date"] = apiObject.GetUploadDate()

	return tfMap
}

func flattenServerCertificates(apiObjects []oscgo.ServerCertificate) []map[string]interface{} {
	if len(apiObjects) == 0 {
		return nil
	}

	var tfList []map[string]interface{}

	for _, apiObject := range apiObjects {

		tfList = append(tfList, flattenServerCertificate(apiObject))
	}

	return tfList
}
